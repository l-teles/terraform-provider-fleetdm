package provider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Minimal fixtures that pass Fleet's checks: the PDF magic bytes, and
// Markdown with visible text and no HTML.
const (
	testEULAPDF1 = "%PDF-1.4\n% terraform-provider-fleetdm test v1\n%%EOF\n"
	testEULAPDF2 = "%PDF-1.4\n% terraform-provider-fleetdm test v2\n%%EOF\n"
	testEULAMD1  = "# Terms\n\nBy enrolling you agree to these terms.\n"
	testEULAMD2  = "# Terms\n\nBy enrolling you agree to these updated terms.\n"
)

// writeEULAFixture writes content to name in dir and returns its path.
func writeEULAFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

type mockEULA struct {
	name    string
	token   string
	content []byte
}

// mockEULAServer is a stateful Fleet EULA API: one slot per platform, 409 on
// a second upload, 404 for a missing EULA or token. legacy mimics Fleet before
// 4.94, which ignores the platform and omits it from metadata.
type mockEULAServer struct {
	mu      sync.Mutex
	legacy  bool
	slots   map[string]*mockEULA
	nextTok int
	posts   int
	deletes int
	// failMetadata makes the next n metadata reads answer 500.
	failMetadata int
}

func newMockEULAServer(t *testing.T, legacy bool) (*mockEULAServer, *httptest.Server) {
	t.Helper()
	m := &mockEULAServer{legacy: legacy, slots: map[string]*mockEULA{}}
	srv := httptest.NewServer(http.HandlerFunc(m.handle))
	t.Cleanup(srv.Close)
	return m, srv
}

func (m *mockEULAServer) put(platform, name, content string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextTok++
	m.slots[platform] = &mockEULA{name: name, token: fmt.Sprintf("tok-%d", m.nextTok), content: []byte(content)}
}

func (m *mockEULAServer) clear(platform string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.slots, platform)
}

func (m *mockEULAServer) counts() (posts, deletes int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.posts, m.deletes
}

func (m *mockEULAServer) platformOf(p string) string {
	if m.legacy || p == "" {
		return "darwin"
	}
	return p
}

func (m *mockEULAServer) handle(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	const base = "/api/v1/fleet/setup_experience/eula"
	switch {
	case r.Method == http.MethodPost && r.URL.Path == base:
		m.posts++
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f, hdr, err := r.FormFile("eula")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		content, _ := io.ReadAll(f)
		_ = f.Close()
		platform := m.platformOf(r.FormValue("platform"))
		if platform == "darwin" && !strings.HasPrefix(string(content), "%PDF") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"Bad request","errors":[{"name":"base","reason":"invalid file type"}]}`))
			return
		}
		if platform == "windows" && strings.Contains(string(content), "<div") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"Bad request","errors":[{"name":"base","reason":"The file contains HTML other than tables and text styles."}]}`))
			return
		}
		if _, ok := m.slots[platform]; ok {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"message":"Resource Already Exists"}`))
			return
		}
		m.nextTok++
		m.slots[platform] = &mockEULA{name: hdr.Filename, token: fmt.Sprintf("tok-%d", m.nextTok), content: content}
		_, _ = w.Write([]byte(`{}`))

	case r.Method == http.MethodGet && r.URL.Path == base+"/metadata" && m.failMetadata > 0:
		m.failMetadata--
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"transient"}`))
	case r.Method == http.MethodGet && r.URL.Path == base+"/metadata":
		platform := m.platformOf(r.URL.Query().Get("platform"))
		e, ok := m.slots[platform]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Resource Not Found"}`))
			return
		}
		sum := sha256.Sum256(e.content)
		body := map[string]any{
			"name":       e.name,
			"token":      e.token,
			"created_at": "2026-01-02T03:04:05Z",
			"sha256":     sum[:], // base64 on the wire, as Fleet's []byte field
		}
		if !m.legacy {
			body["platform"] = platform
		}
		_ = json.NewEncoder(w).Encode(body)

	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, base+"/"):
		token := strings.TrimPrefix(r.URL.Path, base+"/")
		for p, e := range m.slots {
			if e.token == token {
				m.deletes++
				delete(m.slots, p)
				_, _ = w.Write([]byte(`{}`))
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Resource Not Found"}`))

	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/fleet/version":
		version := "4.94.0"
		if m.legacy {
			version = "4.93.0"
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"version": version})

	default:
		http.NotFound(w, r)
	}
}

// checkMockEULAsGone is a CheckDestroy that every slot is empty.
func checkMockEULAsGone(m *mockEULAServer) resource.TestCheckFunc {
	return func(*terraform.State) error {
		m.mu.Lock()
		defer m.mu.Unlock()
		if len(m.slots) != 0 {
			return fmt.Errorf("expected no EULAs after destroy, got %d", len(m.slots))
		}
		return nil
	}
}

func testMockEULAConfig(serverURL, platform, source, name string) string {
	nameLine := ""
	if name != "" {
		nameLine = fmt.Sprintf("  name     = %q\n", name)
	}
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %q
  api_key        = "test-token"
}

resource "fleetdm_mdm_eula" "test" {
  platform = %q
  source   = %q
%s}
`, serverURL, platform, source, nameLine)
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", sum)
}

// checkEULACounts asserts the number of uploads and deletes the mock has seen.
func checkEULACounts(m *mockEULAServer, wantPosts, wantDeletes int) resource.TestCheckFunc {
	return func(*terraform.State) error {
		posts, deletes := m.counts()
		if posts != wantPosts || deletes != wantDeletes {
			return fmt.Errorf("expected %d uploads and %d deletes, got %d and %d", wantPosts, wantDeletes, posts, deletes)
		}
		return nil
	}
}

func TestMDMEULAResource_mockDarwinLifecycle(t *testing.T) {
	m, srv := newMockEULAServer(t, false)
	dir := t.TempDir()
	v1 := writeEULAFixture(t, dir, "terms.pdf", testEULAPDF1)
	moved := writeEULAFixture(t, t.TempDir(), "terms.pdf", testEULAPDF1)
	v2Dir := t.TempDir()
	v2 := writeEULAFixture(t, v2Dir, "terms.pdf", testEULAPDF2)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testMockEULAConfig(srv.URL, "darwin", v1, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_mdm_eula.test", "id", "darwin"),
					resource.TestCheckResourceAttr("fleetdm_mdm_eula.test", "name", "terms.pdf"),
					resource.TestCheckResourceAttr("fleetdm_mdm_eula.test", "sha256", sha256Hex(testEULAPDF1)),
					resource.TestCheckResourceAttr("fleetdm_mdm_eula.test", "token", "tok-1"),
					resource.TestCheckResourceAttr("fleetdm_mdm_eula.test", "created_at", "2026-01-02T03:04:05Z"),
					checkEULACounts(m, 1, 0),
				),
			},
			// Same bytes at another path: an in-place no-op for Fleet.
			{
				Config: testMockEULAConfig(srv.URL, "darwin", moved, ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_mdm_eula.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_mdm_eula.test", "token", "tok-1"),
					checkEULACounts(m, 1, 0),
				),
			},
			// New content: delete, then upload.
			{
				Config: testMockEULAConfig(srv.URL, "darwin", v2, ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_mdm_eula.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_mdm_eula.test", "sha256", sha256Hex(testEULAPDF2)),
					resource.TestCheckResourceAttr("fleetdm_mdm_eula.test", "token", "tok-2"),
					checkEULACounts(m, 2, 1),
				),
			},
			// New name, same content: replaced too.
			{
				Config: testMockEULAConfig(srv.URL, "darwin", v2, "acme.pdf"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_mdm_eula.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_mdm_eula.test", "name", "acme.pdf"),
					checkEULACounts(m, 3, 2),
				),
			},
			{
				ResourceName:            "fleetdm_mdm_eula.test",
				ImportState:             true,
				ImportStateId:           "darwin",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"source"},
			},
		},
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			checkMockEULAsGone(m),
			checkEULACounts(m, 3, 3),
		),
	})
}

func TestMDMEULAResource_mockWindowsLifecycle(t *testing.T) {
	m, srv := newMockEULAServer(t, false)
	v1 := writeEULAFixture(t, t.TempDir(), "source.md", testEULAMD1)
	v2 := writeEULAFixture(t, t.TempDir(), "source.md", testEULAMD2)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testMockEULAConfig(srv.URL, "windows", v1, "Terms.MD"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_mdm_eula.test", "id", "windows"),
					resource.TestCheckResourceAttr("fleetdm_mdm_eula.test", "platform", "windows"),
					resource.TestCheckResourceAttr("fleetdm_mdm_eula.test", "name", "Terms.MD"),
					resource.TestCheckResourceAttr("fleetdm_mdm_eula.test", "sha256", sha256Hex(testEULAMD1)),
					checkEULACounts(m, 1, 0),
				),
			},
			{
				Config: testMockEULAConfig(srv.URL, "windows", v2, "Terms.MD"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_mdm_eula.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_mdm_eula.test", "sha256", sha256Hex(testEULAMD2)),
					checkEULACounts(m, 2, 1),
				),
			},
			{
				ResourceName:            "fleetdm_mdm_eula.test",
				ImportState:             true,
				ImportStateId:           "windows",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"source"},
			},
		},
		CheckDestroy: checkMockEULAsGone(m),
	})
}

// TestMDMEULAResource_mockDrift covers a change made in Fleet (replace) and a
// EULA deleted in Fleet (404 on read: removed from state, then re-created).
func TestMDMEULAResource_mockDrift(t *testing.T) {
	m, srv := newMockEULAServer(t, false)
	src := writeEULAFixture(t, t.TempDir(), "terms.pdf", testEULAPDF1)
	cfg := testMockEULAConfig(srv.URL, "darwin", src, "")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkMockEULAsGone(m),
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				PreConfig: func() {
					m.clear("darwin")
					m.put("darwin", "terms.pdf", testEULAPDF2)
				},
				Config: cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_mdm_eula.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.TestCheckResourceAttr("fleetdm_mdm_eula.test", "sha256", sha256Hex(testEULAPDF1)),
			},
			{
				PreConfig: func() { m.clear("darwin") },
				Config:    cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_mdm_eula.test", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.TestCheckResourceAttr("fleetdm_mdm_eula.test", "sha256", sha256Hex(testEULAPDF1)),
			},
		},
	})
}

func TestMDMEULAResource_mockConflict(t *testing.T) {
	m, srv := newMockEULAServer(t, false)
	m.put("darwin", "existing.pdf", testEULAPDF2)
	src := writeEULAFixture(t, t.TempDir(), "terms.pdf", testEULAPDF1)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testMockEULAConfig(srv.URL, "darwin", src, ""),
				ExpectError: regexp.MustCompile(`(?s)already\s+has\s+a\s+darwin\s+EULA`),
			},
		},
	})
}

// TestMDMEULAResource_mockPre494Windows: Fleet before 4.94 ignores the
// platform, so a Windows EULA must never adopt the macOS one.
func TestMDMEULAResource_mockPre494Windows(t *testing.T) {
	m, srv := newMockEULAServer(t, true)
	m.put("darwin", "mac.pdf", testEULAPDF1)
	src := writeEULAFixture(t, t.TempDir(), "terms.md", testEULAMD1)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:        testMockEULAConfig(srv.URL, "windows", src, ""),
				ResourceName:  "fleetdm_mdm_eula.test",
				ImportState:   true,
				ImportStateId: "windows",
				ExpectError:   regexp.MustCompile(`(?s)does\s+not\s+support\s+Windows\s+EULAs`),
			},
			{
				PreConfig:   func() { m.clear("darwin") },
				Config:      testMockEULAConfig(srv.URL, "windows", src, ""),
				ExpectError: regexp.MustCompile(`(?s)Fleet\s+before\s+4\.94\s+does\s+not\s+support\s+Windows\s+EULAs`),
			},
		},
	})
	if _, deletes := m.counts(); deletes != 0 {
		t.Fatalf("expected no deletes against a pre-4.94 server, got %d", deletes)
	}
}

// TestMDMEULAResource_mock494MarkdownRejected: Fleet's own reason for
// rejecting the Markdown reaches the practitioner.
func TestMDMEULAResource_mock494MarkdownRejected(t *testing.T) {
	m, srv := newMockEULAServer(t, false)
	src := writeEULAFixture(t, t.TempDir(), "terms.md", "# Terms\n\n<div>x</div>\n")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testMockEULAConfig(srv.URL, "windows", src, ""),
				ExpectError: regexp.MustCompile(`(?s)contains\s+HTML`),
			},
		},
	})
	if posts, _ := m.counts(); posts != 1 {
		t.Fatalf("expected one upload attempt, got %d", posts)
	}
}

// TestMDMEULAResource_serverBefore494 checks the version gate on the
// pre-4.94 hint, so a 4.94 rejection is not blamed on the Fleet version.
func TestMDMEULAResource_serverBefore494(t *testing.T) {
	cases := []struct {
		version string
		status  int
		want    bool
	}{
		{"4.93.0", http.StatusOK, true},
		{"v4.80.1-rc1", http.StatusOK, true},
		{"4.94.0", http.StatusOK, false},
		{"5.0.0", http.StatusOK, false},
		{"0.0.0-SNAPSHOT-abc", http.StatusOK, false},
		{"dev", http.StatusOK, false},
		{"", http.StatusInternalServerError, false},
	}
	for _, tc := range cases {
		t.Run(tc.version, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_ = json.NewEncoder(w).Encode(map[string]string{"version": tc.version})
			}))
			defer srv.Close()
			client, err := fleetdm.NewClient(fleetdm.ClientConfig{ServerAddress: srv.URL, APIKey: "test"})
			if err != nil {
				t.Fatal(err)
			}
			r := &mdmEULAResource{client: client}
			if got := r.serverBefore494(t.Context()); got != tc.want {
				t.Errorf("serverBefore494(%q) = %v, want %v", tc.version, got, tc.want)
			}
		})
	}
}

func TestMDMEULAResource_mockPlanValidation(t *testing.T) {
	_, srv := newMockEULAServer(t, false)
	dir := t.TempDir()
	md := writeEULAFixture(t, dir, "terms.md", testEULAMD1)
	txt := writeEULAFixture(t, dir, "terms.txt", testEULAMD1)
	notPDF := writeEULAFixture(t, dir, "terms.pdf", "not a pdf")
	empty := writeEULAFixture(t, dir, "empty.md", " \n\t\n")
	big := writeEULAFixture(t, dir, "big.md", "# T\n\n"+strings.Repeat("a", 512<<10))
	notUTF8 := writeEULAFixture(t, dir, "bad.md", "# T\n\n\xff\xfe\n")

	cases := []struct {
		name   string
		config string
		want   string
	}{
		{"windows name not md", testMockEULAConfig(srv.URL, "windows", txt, ""), `Windows\s+EULA\s+must\s+be\s+a\s+Markdown`},
		{"windows configured name not md", testMockEULAConfig(srv.URL, "windows", md, "terms.pdf"), `Windows\s+EULA\s+must\s+be\s+a\s+Markdown`},
		{"darwin not pdf", testMockEULAConfig(srv.URL, "darwin", notPDF, ""), `macOS\s+EULA\s+must\s+be\s+a\s+PDF`},
		{"windows empty", testMockEULAConfig(srv.URL, "windows", empty, ""), `The\s+file\s+is\s+empty`},
		{"windows too large", testMockEULAConfig(srv.URL, "windows", big, ""), `512\s+KB\s+or\s+smaller`},
		{"windows not utf8", testMockEULAConfig(srv.URL, "windows", notUTF8, ""), `must\s+be\s+UTF-8`},
		{"name with path", testMockEULAConfig(srv.URL, "windows", md, "dir/terms.md"), `must\s+not\s+contain\s+a\s+path\s+separator`},
		{"name too long", testMockEULAConfig(srv.URL, "windows", md, strings.Repeat("a", 253)+".md"), `255\s+characters\s+or\s+fewer`},
		{"bad platform", testMockEULAConfig(srv.URL, "linux", md, ""), `value\s+must\s+be\s+one\s+of`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{Config: tc.config, ExpectError: regexp.MustCompile(`(?s)` + tc.want)},
				},
			})
		})
	}
}

func TestMDMEULAResource_mockInvalidImportID(t *testing.T) {
	_, srv := newMockEULAServer(t, false)
	src := writeEULAFixture(t, t.TempDir(), "terms.pdf", testEULAPDF1)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:        testMockEULAConfig(srv.URL, "darwin", src, ""),
				ResourceName:  "fleetdm_mdm_eula.test",
				ImportState:   true,
				ImportStateId: "macos",
				ExpectError:   regexp.MustCompile(`(?s)Expected\s+the\s+platform`),
			},
		},
	})
}

// Live acceptance tests. Fleet allows one EULA per platform, so these need
// the rig's darwin and windows EULA slots to be empty.

func testAccMDMEULAConfig(platform, source string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_mdm_eula" "test" {
  platform = %q
  source   = %q
}
`, platform, source)
}

func testAccMDMEULALifecycle(t *testing.T, platform, ext, v1Content, v2Content string) {
	suffix := acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	name := "tf-acc-eula-" + suffix + ext
	v1 := writeEULAFixture(t, t.TempDir(), name, v1Content)
	v2 := writeEULAFixture(t, t.TempDir(), name, v2Content)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccMDMEULAConfig(platform, v1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_mdm_eula.test", "id", platform),
					resource.TestCheckResourceAttr("fleetdm_mdm_eula.test", "name", name),
					resource.TestCheckResourceAttr("fleetdm_mdm_eula.test", "sha256", sha256Hex(v1Content)),
					resource.TestCheckResourceAttrSet("fleetdm_mdm_eula.test", "token"),
					resource.TestCheckResourceAttrSet("fleetdm_mdm_eula.test", "created_at"),
				),
			},
			{
				Config: testAccMDMEULAConfig(platform, v2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("fleetdm_mdm_eula.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.TestCheckResourceAttr("fleetdm_mdm_eula.test", "sha256", sha256Hex(v2Content)),
			},
			{
				ResourceName:            "fleetdm_mdm_eula.test",
				ImportState:             true,
				ImportStateId:           platform,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"source"},
			},
		},
		CheckDestroy: testAccCheckMDMEULADestroyed(platform),
	})
}

// testAccCheckMDMEULADestroyed asserts Fleet has no EULA left for the platform.
func testAccCheckMDMEULADestroyed(platform string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		client, err := certificateIntegrationsTestClient()
		if err != nil {
			return err
		}
		_, err = client.GetEULAMetadata(context.Background(), platform)
		if isNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("checking %s EULA after destroy: %w", platform, err)
		}
		return fmt.Errorf("%s EULA still exists after destroy", platform)
	}
}

func TestAccMDMEULAResource_darwin(t *testing.T) {
	testAccMDMEULALifecycle(t, "darwin", ".pdf", testEULAPDF1, testEULAPDF2)
}

func TestAccMDMEULAResource_windows(t *testing.T) {
	testAccMDMEULALifecycle(t, "windows", ".md", testEULAMD1, testEULAMD2)
}

// TestMDMEULAResource_mockReadAfterUploadFails: when the read-back after an
// upload fails, the EULA already exists in Fleet, so the resource must be
// saved (tainted) and replaced on the next apply instead of orphaned, which
// would 409 on every retry.
func TestMDMEULAResource_mockReadAfterUploadFails(t *testing.T) {
	m, srv := newMockEULAServer(t, false)
	src := writeEULAFixture(t, t.TempDir(), "terms.pdf", testEULAPDF1)
	cfg := testMockEULAConfig(srv.URL, "darwin", src, "")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { m.mu.Lock(); m.failMetadata = 1; m.mu.Unlock() },
				Config:      cfg,
				ExpectError: regexp.MustCompile(`(?s)Error\s+reading\s+EULA\s+after\s+upload`),
			},
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_mdm_eula.test", "sha256", sha256Hex(testEULAPDF1)),
					checkEULACounts(m, 2, 1),
				),
			},
		},
	})
}
