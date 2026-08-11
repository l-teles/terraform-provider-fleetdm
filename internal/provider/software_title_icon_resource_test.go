package provider

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"image"
	_ "image/png" // register the PNG decoder for the fixture self-check
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// makeTestPNG builds a real, valid PNG of the requested size in memory rather
// than checking a binary fixture into the repo. Fleet validates icons by
// decoding them (PNG only, 120x120..1024x1024, under 100KB), so live tests
// need genuine image data — a stubbed header would be rejected. A solid-colour
// image compresses to a few hundred bytes even at 120x120.
func makeTestPNG(width, height int, r, g, b byte) []byte {
	chunk := func(typ string, data []byte) []byte {
		var out bytes.Buffer
		length := make([]byte, 4)
		binary.BigEndian.PutUint32(length, uint32(len(data))) // #nosec G115 -- test-controlled sizes
		out.Write(length)
		payload := append([]byte(typ), data...)
		out.Write(payload)
		sum := make([]byte, 4)
		binary.BigEndian.PutUint32(sum, crc32.ChecksumIEEE(payload))
		out.Write(sum)
		return out.Bytes()
	}

	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], uint32(width))  // #nosec G115 -- test-controlled sizes
	binary.BigEndian.PutUint32(ihdr[4:8], uint32(height)) // #nosec G115 -- test-controlled sizes
	ihdr[8] = 8                                           // bit depth
	ihdr[9] = 2                                           // colour type: truecolour RGB
	// ihdr[10..12] stay zero: deflate, no filter, no interlace.

	// Raw scanlines: one filter byte (0 = None) followed by width RGB triples.
	var raw bytes.Buffer
	for y := 0; y < height; y++ {
		raw.WriteByte(0)
		for x := 0; x < width; x++ {
			raw.Write([]byte{r, g, b})
		}
	}
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	_, _ = zw.Write(raw.Bytes())
	_ = zw.Close()

	var png bytes.Buffer
	png.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	png.Write(chunk("IHDR", ihdr))
	png.Write(chunk("IDAT", compressed.Bytes()))
	png.Write(chunk("IEND", nil))
	return png.Bytes()
}

// writeTestPNG writes a generated PNG into t.TempDir() and returns its path.
func writeTestPNG(t *testing.T, name string, width, height int, r, g, b byte) (string, []byte) {
	t.Helper()
	content := makeTestPNG(width, height, r, g, b)
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return p, content
}

// TestMakeTestPNGIsDecodable guards the fixture generator itself: if it ever
// emits something the image package won't decode, the live tests would fail
// against Fleet for a reason that has nothing to do with the provider.
func TestMakeTestPNGIsDecodable(t *testing.T) {
	content := makeTestPNG(120, 120, 0, 128, 255)

	cfg, format, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("generated PNG does not decode: %v", err)
	}
	if format != "png" {
		t.Errorf("expected format png, got %s", format)
	}
	if cfg.Width != 120 || cfg.Height != 120 {
		t.Errorf("expected 120x120, got %dx%d", cfg.Width, cfg.Height)
	}
	// Fleet caps icons at 100KB; a solid-colour 120x120 must be far under.
	if len(content) > 100*1024 {
		t.Errorf("generated PNG is %d bytes, over Fleet's 100KB limit", len(content))
	}
}

// TestReadIconFile covers the local pre-flight guards.
func TestReadIconFile(t *testing.T) {
	dir := t.TempDir()

	validPath := filepath.Join(dir, "ok.png")
	valid := makeTestPNG(120, 120, 1, 2, 3)
	if err := os.WriteFile(validPath, valid, 0o600); err != nil {
		t.Fatal(err)
	}
	notPNGPath := filepath.Join(dir, "bad.png")
	if err := os.WriteFile(notPNGPath, []byte("\xff\xd8\xff\xe0 jpeg-ish"), 0o600); err != nil {
		t.Fatal(err)
	}
	emptyPath := filepath.Join(dir, "empty.png")
	if err := os.WriteFile(emptyPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("valid png", func(t *testing.T) {
		got, err := readIconFile(validPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.Equal(got, valid) {
			t.Error("content mismatch")
		}
	})

	t.Run("empty path", func(t *testing.T) {
		if _, err := readIconFile(""); err == nil {
			t.Error("expected an error for an empty path")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		if _, err := readIconFile(filepath.Join(dir, "nope.png")); err == nil {
			t.Error("expected an error for a missing file")
		}
	})

	t.Run("empty file", func(t *testing.T) {
		if _, err := readIconFile(emptyPath); err == nil {
			t.Error("expected an error for an empty file")
		}
	})

	t.Run("directory", func(t *testing.T) {
		if _, err := readIconFile(dir); err == nil {
			t.Error("expected an error when icon_path is a directory")
		}
	})

	t.Run("over the size bound", func(t *testing.T) {
		// Just past the cap, and deliberately PNG-prefixed: the size check has
		// to fire before the content check, and before the file is buffered.
		bigPath := filepath.Join(dir, "huge.png")
		big := make([]byte, maxIconFileSize+1)
		copy(big, pngMagic)
		if err := os.WriteFile(bigPath, big, 0o600); err != nil {
			t.Fatal(err)
		}

		_, err := readIconFile(bigPath)
		if err == nil {
			t.Fatal("expected an error for a file over the size bound")
		}
		if !bytes.Contains([]byte(err.Error()), []byte("100KB")) {
			t.Errorf("expected the error to cite Fleet's 100KB cap, got: %v", err)
		}
	})

	t.Run("at the size bound", func(t *testing.T) {
		// Exactly at the cap must still be read; only the content check
		// should reject this one, proving the bound is inclusive.
		atPath := filepath.Join(dir, "at-limit.png")
		at := make([]byte, maxIconFileSize)
		copy(at, pngMagic)
		if err := os.WriteFile(atPath, at, 0o600); err != nil {
			t.Fatal(err)
		}

		got, err := readIconFile(atPath)
		if err != nil {
			t.Fatalf("a file at exactly the bound must be read, got: %v", err)
		}
		if len(got) != maxIconFileSize {
			t.Errorf("expected %d bytes, got %d", maxIconFileSize, len(got))
		}
	})

	t.Run("not a png", func(t *testing.T) {
		_, err := readIconFile(notPNGPath)
		if err == nil {
			t.Fatal("expected an error for non-PNG content")
		}
		// The message must name the file — that's the whole reason this guard
		// exists rather than deferring to Fleet's generic rejection.
		if !bytes.Contains([]byte(err.Error()), []byte(notPNGPath)) {
			t.Errorf("expected the path in the error, got: %v", err)
		}
	})
}

func TestTitleIconID(t *testing.T) {
	if got := titleIconID(42, 0); got != "42:0" {
		t.Errorf("expected 42:0, got %s", got)
	}
	if got := titleIconID(7, 3); got != "7:3" {
		t.Errorf("expected 7:3, got %s", got)
	}
}

func TestIconFilename(t *testing.T) {
	tests := []struct {
		name     string
		iconPath types.String
		filename types.String
		want     string
	}{
		{
			name:     "explicit filename wins",
			iconPath: types.StringValue("/tmp/icons/logo.png"),
			filename: types.StringValue("brand.png"),
			want:     "brand.png",
		},
		{
			name:     "derived from path",
			iconPath: types.StringValue("/tmp/icons/logo.png"),
			filename: types.StringUnknown(),
			want:     "logo.png",
		},
		{
			name:     "empty filename falls back to path",
			iconPath: types.StringValue("/tmp/icons/logo.png"),
			filename: types.StringValue(""),
			want:     "logo.png",
		},
		{
			// The import case: neither is known. Must not become ".".
			name:     "nothing known",
			iconPath: types.StringNull(),
			filename: types.StringNull(),
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := iconFilename(tt.iconPath, tt.filename); got != tt.want {
				t.Errorf("iconFilename() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSumHex(t *testing.T) {
	// SHA256 of the empty string, as a fixed anchor.
	if got := sumHex(nil); got != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("unexpected hash for empty input: %s", got)
	}
}

// iconMockServer is a stand-in Fleet that implements the three icon routes
// against an in-memory store, so the mock tests exercise real state
// transitions (upload, drift, delete) instead of fixed canned replies.
type iconMockServer struct {
	mu      sync.Mutex
	icons   map[string][]byte // keyed by "titleID:fleetID"
	uploads int
	deletes int
}

func newIconMockServer() *iconMockServer {
	return &iconMockServer{icons: map[string][]byte{}}
}

func (m *iconMockServer) handler(t *testing.T) http.Handler {
	t.Helper()
	iconPath := regexp.MustCompile(`^/api/v1/fleet/software/titles/(\d+)/icon$`)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		match := iconPath.FindStringSubmatch(r.URL.Path)
		if match != nil {
			fleetID := r.URL.Query().Get("fleet_id")
			if fleetID == "" {
				// Mirror Fleet: the scope must come from the query string.
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"message": "team_id is required"})
				return
			}
			key := match[1] + ":" + fleetID

			m.mu.Lock()
			defer m.mu.Unlock()

			switch r.Method {
			case http.MethodPut:
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				headers := r.MultipartForm.File["icon"]
				if len(headers) == 0 {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"message": "either icon multipart field or hashSHA256 and filename are required",
					})
					return
				}
				f, err := headers[0].Open()
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				buf := new(bytes.Buffer)
				_, _ = buf.ReadFrom(f)
				_ = f.Close()
				m.icons[key] = buf.Bytes()
				m.uploads++
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"icon_url": "/api/latest/fleet/software/titles/" + match[1] + "/icon?fleet_id=" + fleetID,
				})
			case http.MethodGet:
				content, ok := m.icons[key]
				if !ok {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusNotFound)
					_ = json.NewEncoder(w).Encode(map[string]any{"message": "Resource Not Found"})
					return
				}
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write(content)
			case http.MethodDelete:
				if _, ok := m.icons[key]; !ok {
					// Reproduce Fleet's non-idempotent DELETE: 500 with the
					// no-rows sentinel rather than a 404.
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_ = json.NewEncoder(w).Encode(map[string]any{"message": "sql: no rows in result set"})
					return
				}
				delete(m.icons, key)
				m.deletes++
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{}`))
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
			return
		}

		// Title GET, used only for icon_url on refresh.
		if r.URL.Path == "/api/v1/fleet/software/titles/42" && r.Method == http.MethodGet {
			m.mu.Lock()
			_, has := m.icons["42:"+r.URL.Query().Get("team_id")]
			m.mu.Unlock()
			iconURL := ""
			if has {
				iconURL = "/api/latest/fleet/software/titles/42/icon?fleet_id=" + r.URL.Query().Get("team_id")
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"software_title": map[string]any{
					"id":       42,
					"name":     "test-app.sh",
					"source":   "sh_packages",
					"icon_url": iconURL,
					"software_package": map[string]any{
						"title_id": 42,
						"platform": "linux",
					},
				},
			})
			return
		}

		http.NotFound(w, r)
	})
}

func (m *iconMockServer) stats() (uploads, deletes int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.uploads, m.deletes
}

func testAccSoftwareTitleIconConfig(serverURL, iconPath string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_title_icon" "test" {
  title_id  = 42
  fleet_id  = 0
  icon_path = %[2]q
}
`, serverURL, iconPath)
}

// TestAccSoftwareTitleIconResource_basic covers create against a mock Fleet:
// the icon is uploaded, hash_sha256 matches the local file, and icon_url is
// captured from the response.
func TestAccSoftwareTitleIconResource_basic(t *testing.T) {
	iconPath, content := writeTestPNG(t, "icon.png", 120, 120, 0, 128, 255)
	wantHash := hex.EncodeToString(sumOf(content))

	mock := newIconMockServer()
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSoftwareTitleIconConfig(server.URL, iconPath),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_title_icon.test", "id", "42:0"),
					resource.TestCheckResourceAttr("fleetdm_software_title_icon.test", "title_id", "42"),
					resource.TestCheckResourceAttr("fleetdm_software_title_icon.test", "fleet_id", "0"),
					resource.TestCheckResourceAttr("fleetdm_software_title_icon.test", "filename", "icon.png"),
					resource.TestCheckResourceAttr("fleetdm_software_title_icon.test", "hash_sha256", wantHash),
					resource.TestCheckResourceAttr("fleetdm_software_title_icon.test", "icon_url",
						"/api/latest/fleet/software/titles/42/icon?fleet_id=0"),
				),
			},
		},
	})

	uploads, _ := mock.stats()
	if uploads != 1 {
		t.Errorf("expected exactly 1 upload, got %d", uploads)
	}
}

// TestAccSoftwareTitleIconResource_replaceImage changes the file content and
// asserts the icon is re-uploaded in place — same resource ID, no destroy.
func TestAccSoftwareTitleIconResource_replaceImage(t *testing.T) {
	dir := t.TempDir()
	iconPath := filepath.Join(dir, "icon.png")
	first := makeTestPNG(120, 120, 0, 128, 255)
	second := makeTestPNG(200, 200, 255, 64, 0)
	if err := os.WriteFile(iconPath, first, 0o600); err != nil {
		t.Fatal(err)
	}

	mock := newIconMockServer()
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	// Counters are sampled inside the second step's Check, i.e. after the
	// replace but before the framework's terminal destroy — which issues its
	// own DELETE and would otherwise mask the thing being asserted.
	var uploadsAfterReplace, deletesAfterReplace int

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSoftwareTitleIconConfig(server.URL, iconPath),
				Check: resource.TestCheckResourceAttr("fleetdm_software_title_icon.test",
					"hash_sha256", hex.EncodeToString(sumOf(first))),
			},
			{
				// Same path, different bytes: swap the file between steps.
				PreConfig: func() {
					if err := os.WriteFile(iconPath, second, 0o600); err != nil {
						t.Fatal(err)
					}
				},
				Config: testAccSoftwareTitleIconConfig(server.URL, iconPath),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_title_icon.test",
						"hash_sha256", hex.EncodeToString(sumOf(second))),
					resource.TestCheckResourceAttr("fleetdm_software_title_icon.test", "id", "42:0"),
					func(_ *terraform.State) error {
						uploadsAfterReplace, deletesAfterReplace = mock.stats()
						return nil
					},
				),
			},
		},
	})

	if uploadsAfterReplace != 2 {
		t.Errorf("expected 2 uploads (create + replace), got %d", uploadsAfterReplace)
	}
	// The replace must go through PUT alone. A DELETE here would mean the
	// resource briefly left the title with no icon.
	if deletesAfterReplace != 0 {
		t.Errorf("expected no DELETE during an in-place image replace, got %d", deletesAfterReplace)
	}
}

// TestAccSoftwareTitleIconResource_filenameOnlyChange asserts that changing
// only `filename` still re-uploads. Fleet echoes the upload filename in the
// icon download's Content-Disposition, so it is server-visible state rather
// than a provider-local label — skipping the call would leave Fleet serving
// the old name while state claimed the new one.
func TestAccSoftwareTitleIconResource_filenameOnlyChange(t *testing.T) {
	iconPath, _ := writeTestPNG(t, "icon.png", 120, 120, 0, 128, 255)

	mock := newIconMockServer()
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	config := func(filename string) string {
		return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_title_icon" "test" {
  title_id  = 42
  fleet_id  = 0
  icon_path = %[2]q
  filename  = %[3]q
}
`, server.URL, iconPath, filename)
	}

	var uploadsAfterRename int

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config("first.png"),
				Check: resource.TestCheckResourceAttr("fleetdm_software_title_icon.test",
					"filename", "first.png"),
			},
			{
				Config: config("second.png"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_title_icon.test",
						"filename", "second.png"),
					func(_ *terraform.State) error {
						uploadsAfterRename, _ = mock.stats()
						return nil
					},
				),
			},
			{
				// Re-applying the same config must not re-upload.
				Config:   config("second.png"),
				PlanOnly: true,
			},
		},
	})

	if uploadsAfterRename != 2 {
		t.Errorf("expected the rename to re-upload (2 uploads), got %d", uploadsAfterRename)
	}
}

// TestAccSoftwareTitleIconResource_derivedFilenameIsSticky pins the behaviour
// the `filename` attribute description warns about.
//
// With `filename` omitted it is Computed, so UseStateForUnknown carries the
// prior value into every later plan — the derived name is fixed at creation
// and a subsequent icon_path rename does NOT re-derive it. That is standard
// framework behaviour for an Optional+Computed attribute and not something the
// resource should fight (re-deriving would fire a diff whenever a user moved
// the file), but it does contradict a naive reading of "defaults to the base
// name of icon_path", hence the note in the docs and this test.
func TestAccSoftwareTitleIconResource_derivedFilenameIsSticky(t *testing.T) {
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "original-name.png")
	secondPath := filepath.Join(dir, "renamed.png")
	content := makeTestPNG(120, 120, 0, 128, 255)
	if err := os.WriteFile(firstPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, content, 0o600); err != nil {
		t.Fatal(err)
	}

	mock := newIconMockServer()
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSoftwareTitleIconConfig(server.URL, firstPath),
				Check: resource.TestCheckResourceAttr("fleetdm_software_title_icon.test",
					"filename", "original-name.png"),
			},
			{
				// Same bytes, different path. filename stays as first derived.
				Config: testAccSoftwareTitleIconConfig(server.URL, secondPath),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_title_icon.test",
						"filename", "original-name.png"),
					resource.TestCheckResourceAttr("fleetdm_software_title_icon.test",
						"icon_path", secondPath),
				),
			},
		},
	})
}

// TestAccSoftwareTitleIconResource_rejectsEmptyFilename pins the validator on
// `filename`.
//
// An empty string can't round-trip: the upload needs a non-empty part
// filename, so the provider would substitute the icon_path base name and hand
// back a value the plan never contained — Terraform then aborts the apply with
// "Provider produced inconsistent result after apply", which points at the
// provider rather than at the config. Rejecting "" at validate time gives the
// user an error naming the attribute instead.
func TestAccSoftwareTitleIconResource_rejectsEmptyFilename(t *testing.T) {
	iconPath, _ := writeTestPNG(t, "icon.png", 120, 120, 0, 128, 255)

	mock := newIconMockServer()
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	config := fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_title_icon" "test" {
  title_id  = 42
  fleet_id  = 0
  icon_path = %[2]q
  filename  = ""
}
`, server.URL, iconPath)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`(?s)filename.*at least 1`),
			},
		},
	})

	// Validation happens before any API call.
	uploads, _ := mock.stats()
	if uploads != 0 {
		t.Errorf("expected no upload for an invalid config, got %d", uploads)
	}
}

// TestAccSoftwareTitleIconResource_driftRestoresIcon simulates someone
// replacing the icon in the Fleet UI. Because Read hashes the bytes Fleet
// serves back, the change is real drift and the next apply re-uploads.
func TestAccSoftwareTitleIconResource_driftRestoresIcon(t *testing.T) {
	iconPath, content := writeTestPNG(t, "icon.png", 120, 120, 0, 128, 255)
	wantHash := hex.EncodeToString(sumOf(content))

	mock := newIconMockServer()
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSoftwareTitleIconConfig(server.URL, iconPath),
				Check: resource.TestCheckResourceAttr("fleetdm_software_title_icon.test",
					"hash_sha256", wantHash),
			},
			{
				// Out-of-band edit, then re-apply the unchanged config.
				PreConfig: func() {
					mock.mu.Lock()
					mock.icons["42:0"] = makeTestPNG(150, 150, 9, 9, 9)
					mock.mu.Unlock()
				},
				Config: testAccSoftwareTitleIconConfig(server.URL, iconPath),
				Check: resource.TestCheckResourceAttr("fleetdm_software_title_icon.test",
					"hash_sha256", wantHash),
			},
		},
	})

	uploads, _ := mock.stats()
	if uploads != 2 {
		t.Errorf("expected the drifted icon to be re-uploaded (2 uploads), got %d", uploads)
	}
}

// TestAccSoftwareTitleIconResource_recreateOnTitleChange asserts title_id and
// fleet_id are ForceNew: there is no Fleet operation that moves an icon
// between titles or fleets.
func TestAccSoftwareTitleIconResource_recreateOnTitleChange(t *testing.T) {
	iconPath, _ := writeTestPNG(t, "icon.png", 120, 120, 0, 128, 255)

	mock := newIconMockServer()
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	config := func(fleetID int) string {
		return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_title_icon" "test" {
  title_id  = 42
  fleet_id  = %[2]d
  icon_path = %[3]q
}
`, server.URL, fleetID, iconPath)
	}

	// Sampled after the recreate, before the framework's terminal destroy.
	var uploadsAfterRecreate, deletesAfterRecreate int

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config(0),
				Check:  resource.TestCheckResourceAttr("fleetdm_software_title_icon.test", "id", "42:0"),
			},
			{
				Config: config(3),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_title_icon.test", "id", "42:3"),
					func(_ *terraform.State) error {
						uploadsAfterRecreate, deletesAfterRecreate = mock.stats()
						// The icon must have moved fleets, not been duplicated.
						mock.mu.Lock()
						defer mock.mu.Unlock()
						if _, stale := mock.icons["42:0"]; stale {
							return fmt.Errorf("icon for the old fleet_id=0 scope was left behind")
						}
						if _, ok := mock.icons["42:3"]; !ok {
							return fmt.Errorf("icon was not created for fleet_id=3")
						}
						return nil
					},
				),
			},
		},
	})

	if uploadsAfterRecreate != 2 {
		t.Errorf("expected 2 uploads across the recreate, got %d", uploadsAfterRecreate)
	}
	// The old fleet's icon must be cleaned up by the replace's destroy leg.
	if deletesAfterRecreate != 1 {
		t.Errorf("expected the fleet_id=0 icon to be deleted on recreate, got %d deletes", deletesAfterRecreate)
	}
}

// TestAccSoftwareTitleIconResource_removedOutOfBand checks that an icon
// deleted in Fleet drops out of state instead of erroring on refresh.
func TestAccSoftwareTitleIconResource_removedOutOfBand(t *testing.T) {
	iconPath, _ := writeTestPNG(t, "icon.png", 120, 120, 0, 128, 255)

	mock := newIconMockServer()
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSoftwareTitleIconConfig(server.URL, iconPath),
			},
			{
				PreConfig: func() {
					mock.mu.Lock()
					delete(mock.icons, "42:0")
					mock.mu.Unlock()
				},
				Config: testAccSoftwareTitleIconConfig(server.URL, iconPath),
				Check: resource.TestCheckResourceAttr("fleetdm_software_title_icon.test",
					"icon_url", "/api/latest/fleet/software/titles/42/icon?fleet_id=0"),
			},
		},
	})

	uploads, _ := mock.stats()
	if uploads != 2 {
		t.Errorf("expected the removed icon to be re-created (2 uploads), got %d", uploads)
	}
}

// TestAccSoftwareTitleIconResource_import covers the composite import ID.
func TestAccSoftwareTitleIconResource_import(t *testing.T) {
	iconPath, _ := writeTestPNG(t, "icon.png", 120, 120, 0, 128, 255)

	mock := newIconMockServer()
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSoftwareTitleIconConfig(server.URL, iconPath),
			},
			{
				ResourceName:  "fleetdm_software_title_icon.test",
				ImportState:   true,
				ImportStateId: "42:0",
				// icon_path is not recoverable from Fleet, so the imported
				// state legitimately differs from the applied one there.
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"icon_path", "filename"},
			},
		},
	})
}

// TestAccSoftwareTitleIconResource_importInvalidID checks the import ID
// diagnostics, including the single-part form that other software resources
// accept but this one cannot (fleet_id has no safe default).
func TestAccSoftwareTitleIconResource_importInvalidID(t *testing.T) {
	iconPath, _ := writeTestPNG(t, "icon.png", 120, 120, 0, 128, 255)

	mock := newIconMockServer()
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	for name, id := range map[string]string{
		"title only":        "42",
		"three parts":       "42:0:1",
		"non-numeric title": "abc:0",
		"non-numeric fleet": "42:abc",
	} {
		t.Run(name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:        testAccSoftwareTitleIconConfig(server.URL, iconPath),
						ResourceName:  "fleetdm_software_title_icon.test",
						ImportState:   true,
						ImportStateId: id,
						ExpectError:   regexp.MustCompile(`(?s)Invalid (import ID|title ID|fleet ID)`),
					},
				},
			})
		})
	}
}

// TestAccSoftwareTitleIconResource_rejectsNonPNG asserts the local guard fires
// with the file path, before any request reaches Fleet.
func TestAccSoftwareTitleIconResource_rejectsNonPNG(t *testing.T) {
	dir := t.TempDir()
	badPath := filepath.Join(dir, "logo.jpg")
	if err := os.WriteFile(badPath, []byte("\xff\xd8\xff\xe0 not a png"), 0o600); err != nil {
		t.Fatal(err)
	}

	mock := newIconMockServer()
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccSoftwareTitleIconConfig(server.URL, badPath),
				ExpectError: regexp.MustCompile(`is not a PNG file`),
			},
		},
	})

	uploads, _ := mock.stats()
	if uploads != 0 {
		t.Errorf("expected no upload attempt for a non-PNG file, got %d", uploads)
	}
}

// TestAccSoftwareTitleIconResource_serverRejection surfaces Fleet's own
// validation message (the pixel-size rule the local guard can't check without
// decoding).
func TestAccSoftwareTitleIconResource_serverRejection(t *testing.T) {
	iconPath, _ := writeTestPNG(t, "tiny.png", 8, 8, 1, 2, 3)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "Bad request",
			"errors":  []map[string]string{{"name": "base", "reason": "icon must be at least 120x120 pixels"}},
		})
	}))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccSoftwareTitleIconConfig(server.URL, iconPath),
				ExpectError: regexp.MustCompile(`at least 120x120 pixels`),
			},
		},
	})
}

// TestAccSoftwareTitleIconResource_live exercises the real Fleet 4.90 icon
// lifecycle: upload, in-place replace, drift restore, and delete.
//
// The backing software title is minted directly through the API client rather
// than through fleetdm_software_custom_package so that a Fleet without a
// working file store can be detected and skipped cleanly, and so the title is
// removed by t.Cleanup even if the Terraform run fails partway. Uploading an
// installer needs Fleet's S3/file store configured; on a rig without it the
// upload fails and this test skips rather than reporting a provider bug.
func TestAccSoftwareTitleIconResource_live(t *testing.T) {
	testAccPreCheck(t)

	client, err := fleetdm.NewClient(fleetdm.ClientConfig{
		ServerAddress: os.Getenv("FLEETDM_URL"),
		APIKey:        os.Getenv("FLEETDM_API_TOKEN"),
		VerifyTLS:     true,
	})
	if err != nil {
		t.Fatalf("failed to build a Fleet client: %v", err)
	}

	ctx := context.Background()
	suffix := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	installerName := "tf-acc-icon-" + suffix + ".sh"

	// fleet_id 0 is the "No team" fleet; the installer is uploaded there too
	// so the two scopes line up.
	const fleetID = 0

	title, err := client.UploadSoftwarePackage(ctx, &fleetdm.UploadSoftwarePackageRequest{
		Software:        []byte("#!/bin/sh\necho tf-acc-icon " + suffix + "\n"),
		Filename:        installerName,
		InstallScript:   "echo install",
		UninstallScript: "echo uninstall",
		SelfService:     true,
	})
	if err != nil {
		t.Skipf("could not mint a backing software title (Fleet needs a working file store for installer uploads): %v", err)
	}

	teamID := fleetID
	t.Cleanup(func() {
		// Remove the title; this takes any remaining icon with it.
		if err := client.DeleteSoftwarePackage(context.Background(), title.ID, &teamID); err != nil {
			t.Logf("cleanup: could not delete software title %d: %v", title.ID, err)
		}
	})

	dir := t.TempDir()
	iconPath := filepath.Join(dir, "icon.png")
	first := makeTestPNG(120, 120, 0, 128, 255)
	second := makeTestPNG(256, 256, 240, 90, 20)
	if err := os.WriteFile(iconPath, first, 0o600); err != nil {
		t.Fatal(err)
	}

	config := providerConfig() + fmt.Sprintf(`
resource "fleetdm_software_title_icon" "test" {
  title_id  = %d
  fleet_id  = %d
  icon_path = %q
}
`, title.ID, fleetID, iconPath)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_title_icon.test", "hash_sha256",
						hex.EncodeToString(sumOf(first))),
					resource.TestCheckResourceAttr("fleetdm_software_title_icon.test", "id",
						fmt.Sprintf("%d:%d", title.ID, fleetID)),
					resource.TestCheckResourceAttrSet("fleetdm_software_title_icon.test", "icon_url"),
					// Confirm against Fleet directly, not just against state:
					// the bytes it serves must be the ones we uploaded.
					func(_ *terraform.State) error {
						got, err := client.GetTitleIcon(ctx, title.ID, fleetID)
						if err != nil {
							return fmt.Errorf("reading the icon back from Fleet: %w", err)
						}
						if !bytes.Equal(got, first) {
							return fmt.Errorf("Fleet served %d bytes, expected the %d uploaded", len(got), len(first))
						}
						return nil
					},
				),
			},
			{
				// In-place replace with a differently-sized image.
				PreConfig: func() {
					if err := os.WriteFile(iconPath, second, 0o600); err != nil {
						t.Fatal(err)
					}
				},
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_title_icon.test", "hash_sha256",
						hex.EncodeToString(sumOf(second))),
					func(_ *terraform.State) error {
						got, err := client.GetTitleIcon(ctx, title.ID, fleetID)
						if err != nil {
							return fmt.Errorf("reading the replaced icon: %w", err)
						}
						if !bytes.Equal(got, second) {
							return fmt.Errorf("Fleet did not serve the replacement icon")
						}
						return nil
					},
				),
			},
			{
				// Out-of-band deletion must be detected and healed.
				PreConfig: func() {
					if err := client.DeleteTitleIcon(ctx, title.ID, fleetID); err != nil {
						t.Fatalf("out-of-band delete failed: %v", err)
					}
				},
				Config: config,
				Check: func(_ *terraform.State) error {
					got, err := client.GetTitleIcon(ctx, title.ID, fleetID)
					if err != nil {
						return fmt.Errorf("icon was not restored after out-of-band deletion: %w", err)
					}
					if !bytes.Equal(got, second) {
						return fmt.Errorf("restored icon does not match the configured one")
					}
					return nil
				},
			},
			{
				ResourceName:            "fleetdm_software_title_icon.test",
				ImportState:             true,
				ImportStateId:           fmt.Sprintf("%d:%d", title.ID, fleetID),
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"icon_path", "filename"},
			},
		},
	})

	// Terraform's own destroy ran at the end of resource.Test; the icon must
	// be gone from Fleet, not merely from state.
	if _, err := client.GetTitleIcon(ctx, title.ID, fleetID); err == nil {
		t.Error("icon still present in Fleet after destroy")
	} else if !fleetdm.IsTitleIconAbsent(err) {
		t.Errorf("unexpected error checking the icon was destroyed: %v", err)
	}
}

// TestAccSoftwareTitleIconResource_liveDeleteIdempotency pins the Fleet quirk
// the Delete path absorbs, against the real server: DELETE on a title with no
// icon answers HTTP 500 "sql: no rows in result set" rather than a 404 or a
// no-op 200.
//
// This is deliberately a client-level test rather than a Terraform one.
// Driving it through resource.Test is not possible: removing the icon
// out-of-band makes the following refresh drop the resource from state, so
// Delete is never reached and the harness fails on a non-empty refresh plan
// instead. The tolerance still matters in the field — `terraform destroy
// -refresh=false`, or the icon disappearing between refresh and destroy — and
// this test plus the mock server's reproduction of the same 500 cover it.
func TestAccSoftwareTitleIconResource_liveDeleteIdempotency(t *testing.T) {
	testAccPreCheck(t)

	client, err := fleetdm.NewClient(fleetdm.ClientConfig{
		ServerAddress: os.Getenv("FLEETDM_URL"),
		APIKey:        os.Getenv("FLEETDM_API_TOKEN"),
		VerifyTLS:     true,
	})
	if err != nil {
		t.Fatalf("failed to build a Fleet client: %v", err)
	}

	ctx := context.Background()
	suffix := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	const fleetID = 0

	title, err := client.UploadSoftwarePackage(ctx, &fleetdm.UploadSoftwarePackageRequest{
		Software:        []byte("#!/bin/sh\necho tf-acc-icon-idem " + suffix + "\n"),
		Filename:        "tf-acc-icon-idem-" + suffix + ".sh",
		InstallScript:   "echo install",
		UninstallScript: "echo uninstall",
	})
	if err != nil {
		t.Skipf("could not mint a backing software title (Fleet needs a working file store): %v", err)
	}

	teamID := fleetID
	t.Cleanup(func() {
		if err := client.DeleteSoftwarePackage(context.Background(), title.ID, &teamID); err != nil {
			t.Logf("cleanup: could not delete software title %d: %v", title.ID, err)
		}
	})

	// The title is brand new, so it has no icon yet. If Fleet ever fixes the
	// non-idempotent DELETE this logs the change rather than silently passing,
	// which is the signal to drop the workaround.
	err = client.DeleteTitleIcon(ctx, title.ID, fleetID)
	switch {
	case err == nil:
		t.Log("note: Fleet now accepts DELETE on a title with no icon; the IsTitleIconAbsent 500 workaround may no longer be needed")
	case !fleetdm.IsTitleIconAbsent(err):
		t.Errorf("deleting a non-existent icon returned an error the provider would not tolerate: %v", err)
	}

	// Now the full round trip: upload, delete (real 200), delete again (the
	// quirk) — the second delete must be absorbed, which is exactly what the
	// resource's Delete relies on.
	if _, err := client.UploadTitleIcon(ctx, &fleetdm.UploadTitleIconRequest{
		TitleID:  title.ID,
		FleetID:  fleetID,
		Icon:     makeTestPNG(120, 120, 3, 3, 3),
		Filename: "icon.png",
	}); err != nil {
		t.Fatalf("uploading the icon failed: %v", err)
	}
	if err := client.DeleteTitleIcon(ctx, title.ID, fleetID); err != nil {
		t.Fatalf("first delete should succeed outright: %v", err)
	}
	if err := client.DeleteTitleIcon(ctx, title.ID, fleetID); err == nil {
		t.Log("note: Fleet's second DELETE now succeeds; the workaround may no longer be needed")
	} else if !fleetdm.IsTitleIconAbsent(err) {
		t.Errorf("second delete returned an intolerable error: %v", err)
	}
}
