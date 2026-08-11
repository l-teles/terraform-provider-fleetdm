package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// webAppMockServer returns an httptest server standing in for Fleet's
// POST /software/web_apps. It records what the provider sent so the tests can
// pin the multipart shape, and answers with the given package name.
// All access is mutex-protected for the testing terraform-plugin harness which
// runs handlers from separate goroutines. Matches s3Mock and
// fakeFleetSoftwareServer elsewhere in this package.
type webAppMockServer struct {
	*httptest.Server

	mu        sync.Mutex
	calls     int
	lastTitle string
	lastURL   string
	lastIcon  []byte
	lastName  string
}

// snapshot returns the recorded request state under the lock, for assertions
// made from the test goroutine after resource.Test returns.
func (m *webAppMockServer) snapshot() (calls int, title, url, iconName string, icon []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls, m.lastTitle, m.lastURL, m.lastName, m.lastIcon
}

func newWebAppMockServer(t *testing.T, appStoreIDs ...string) *webAppMockServer {
	t.Helper()
	m := &webAppMockServer{}
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path != "/api/v1/fleet/software/web_apps" || r.Method != http.MethodPost {
			// Any other path proves the provider tried to read or delete the
			// web app, which Fleet does not support.
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}

		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Errorf("failed to parse multipart form: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		title := r.FormValue("title")
		reqURL := r.FormValue("url")

		var iconName string
		var iconBytes []byte
		if fhs := r.MultipartForm.File["icon"]; len(fhs) > 0 {
			iconName = fhs[0].Filename
			f, err := fhs[0].Open()
			if err != nil {
				t.Errorf("failed to open icon part: %v", err)
			} else {
				defer f.Close()
				iconBytes, err = io.ReadAll(f)
				if err != nil {
					t.Errorf("failed to read icon part: %v", err)
				}
			}
		}

		m.mu.Lock()
		m.lastTitle = title
		m.lastURL = reqURL
		m.lastName = iconName
		m.lastIcon = iconBytes
		id := "com.google.enterprise.webapp.generated"
		if m.calls < len(appStoreIDs) {
			id = appStoreIDs[m.calls]
		}
		m.calls++
		m.mu.Unlock()

		_ = json.NewEncoder(w).Encode(map[string]string{"app_store_id": id})
	}))
	t.Cleanup(m.Close)
	return m
}

func testAccSoftwareWebAppConfig(serverURL, title, url string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_web_app" "test" {
  title = %[2]q
  url   = %[3]q
}
`, serverURL, title, url)
}

// TestAccSoftwareWebAppResource_basic covers create against the mock and
// verifies app_store_id / id are populated from the response. It also proves
// the resource never issues a read or delete request: the mock fails the test
// on any request other than POST /software/web_apps, and Terraform's
// destroy-at-end still has to pass.
func TestAccSoftwareWebAppResource_basic(t *testing.T) {
	const wantID = "com.google.enterprise.webapp.0123456789abcdef"
	m := newWebAppMockServer(t, wantID)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSoftwareWebAppConfig(m.URL, "Support Portal", "https://support.example.com"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_web_app.test", "title", "Support Portal"),
					resource.TestCheckResourceAttr("fleetdm_software_web_app.test", "url", "https://support.example.com"),
					resource.TestCheckResourceAttr("fleetdm_software_web_app.test", "app_store_id", wantID),
					resource.TestCheckResourceAttr("fleetdm_software_web_app.test", "id", wantID),
					resource.TestCheckNoResourceAttr("fleetdm_software_web_app.test", "icon_path"),
				),
			},
		},
	})

	calls, title, gotURL, _, icon := m.snapshot()
	if calls != 1 {
		t.Errorf("expected exactly 1 POST, got %d", calls)
	}
	if title != "Support Portal" {
		t.Errorf("expected title 'Support Portal', got %q", title)
	}
	if gotURL != "https://support.example.com" {
		t.Errorf("expected url 'https://support.example.com', got %q", gotURL)
	}
	if icon != nil {
		t.Errorf("expected no icon part, got %d bytes", len(icon))
	}
}

// TestAccSoftwareWebAppResource_withIcon verifies icon_path is read off disk
// and sent as the "icon" file part under its basename.
func TestAccSoftwareWebAppResource_withIcon(t *testing.T) {
	m := newWebAppMockServer(t, "com.google.enterprise.webapp.icon1")

	iconDir := t.TempDir()
	iconPath := filepath.Join(iconDir, "timesheets-512.png")
	iconBytes := []byte("fake-png-bytes")
	if err := os.WriteFile(iconPath, iconBytes, 0o600); err != nil {
		t.Fatalf("failed to write icon fixture: %v", err)
	}

	cfg := fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_web_app" "test" {
  title     = "Timesheets"
  url       = "https://timesheets.example.com"
  icon_path = %[2]q
}
`, m.URL, iconPath)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_web_app.test", "icon_path", iconPath),
					resource.TestCheckResourceAttr("fleetdm_software_web_app.test", "app_store_id", "com.google.enterprise.webapp.icon1"),
				),
			},
		},
	})

	_, _, _, gotIconName, gotIcon := m.snapshot()
	if gotIconName != "timesheets-512.png" {
		t.Errorf("expected icon filename 'timesheets-512.png', got %q", gotIconName)
	}
	if string(gotIcon) != string(iconBytes) {
		t.Errorf("unexpected icon bytes %q", string(gotIcon))
	}
}

// TestAccSoftwareWebAppResource_titleForcesReplace confirms that changing
// title replaces the resource — Fleet has no update endpoint, so a second POST
// with a fresh app_store_id is the only correct behaviour.
func TestAccSoftwareWebAppResource_titleForcesReplace(t *testing.T) {
	m := newWebAppMockServer(t,
		"com.google.enterprise.webapp.first",
		"com.google.enterprise.webapp.second",
	)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSoftwareWebAppConfig(m.URL, "First", "https://example.com"),
				Check: resource.TestCheckResourceAttr(
					"fleetdm_software_web_app.test", "app_store_id", "com.google.enterprise.webapp.first"),
			},
			{
				Config: testAccSoftwareWebAppConfig(m.URL, "Second", "https://example.com"),
				Check: resource.TestCheckResourceAttr(
					"fleetdm_software_web_app.test", "app_store_id", "com.google.enterprise.webapp.second"),
			},
		},
	})

	if calls, _, _, _, _ := m.snapshot(); calls != 2 {
		t.Errorf("expected 2 POSTs (create + replace), got %d", calls)
	}
}

// TestAccSoftwareWebAppResource_urlForcesReplace is the same guarantee for url.
func TestAccSoftwareWebAppResource_urlForcesReplace(t *testing.T) {
	m := newWebAppMockServer(t,
		"com.google.enterprise.webapp.urlone",
		"com.google.enterprise.webapp.urltwo",
	)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSoftwareWebAppConfig(m.URL, "Portal", "https://one.example.com"),
				Check: resource.TestCheckResourceAttr(
					"fleetdm_software_web_app.test", "app_store_id", "com.google.enterprise.webapp.urlone"),
			},
			{
				Config: testAccSoftwareWebAppConfig(m.URL, "Portal", "https://two.example.com"),
				Check: resource.TestCheckResourceAttr(
					"fleetdm_software_web_app.test", "app_store_id", "com.google.enterprise.webapp.urltwo"),
			},
		},
	})

	if calls, _, _, _, _ := m.snapshot(); calls != 2 {
		t.Errorf("expected 2 POSTs (create + replace), got %d", calls)
	}
}

// TestAccSoftwareWebAppResource_iconPathForcesReplace exercises the
// RequiresReplace on icon_path across all three transitions: adding an icon to
// a web app that had none, swapping it for a different file, and removing it
// again (known -> null).
func TestAccSoftwareWebAppResource_iconPathForcesReplace(t *testing.T) {
	m := newWebAppMockServer(t,
		"com.google.enterprise.webapp.noicon",
		"com.google.enterprise.webapp.icona",
		"com.google.enterprise.webapp.iconb",
		"com.google.enterprise.webapp.iconremoved",
	)

	iconDir := t.TempDir()
	iconA := filepath.Join(iconDir, "a-512.png")
	iconB := filepath.Join(iconDir, "b-512.png")
	if err := os.WriteFile(iconA, []byte("png-a"), 0o600); err != nil {
		t.Fatalf("failed to write icon A: %v", err)
	}
	if err := os.WriteFile(iconB, []byte("png-b"), 0o600); err != nil {
		t.Fatalf("failed to write icon B: %v", err)
	}

	withIcon := func(iconPath string) string {
		return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_web_app" "test" {
  title     = "Portal"
  url       = "https://portal.example.com"
  icon_path = %[2]q
}
`, m.URL, iconPath)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// No icon at all.
				Config: testAccSoftwareWebAppConfig(m.URL, "Portal", "https://portal.example.com"),
				Check: resource.TestCheckResourceAttr(
					"fleetdm_software_web_app.test", "app_store_id", "com.google.enterprise.webapp.noicon"),
			},
			{
				// null -> known forces replacement.
				Config: withIcon(iconA),
				Check: resource.TestCheckResourceAttr(
					"fleetdm_software_web_app.test", "app_store_id", "com.google.enterprise.webapp.icona"),
			},
			{
				// known -> different known forces replacement.
				Config: withIcon(iconB),
				Check: resource.TestCheckResourceAttr(
					"fleetdm_software_web_app.test", "app_store_id", "com.google.enterprise.webapp.iconb"),
			},
			{
				// known -> null forces replacement.
				Config: testAccSoftwareWebAppConfig(m.URL, "Portal", "https://portal.example.com"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"fleetdm_software_web_app.test", "app_store_id", "com.google.enterprise.webapp.iconremoved"),
					resource.TestCheckNoResourceAttr("fleetdm_software_web_app.test", "icon_path"),
				),
			},
		},
	})

	calls, _, _, gotIconName, gotIcon := m.snapshot()
	if calls != 4 {
		t.Errorf("expected 4 POSTs (create + 3 replaces), got %d", calls)
	}
	// The final apply had no icon_path, so the last request must carry no file part.
	if gotIconName != "" || gotIcon != nil {
		t.Errorf("expected no icon part on the final create, got name %q and %d bytes", gotIconName, len(gotIcon))
	}
}

// TestAccSoftwareWebAppResource_androidMDMDisabled covers the gating error the
// VerifyAndroidMDM middleware returns, and checks the diagnostic points the
// user at the prerequisite.
func TestAccSoftwareWebAppResource_androidMDMDisabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Android MDM isn't turned on."})
	}))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSoftwareWebAppConfig(server.URL, "Support Portal", "https://support.example.com"),
				// Assert Fleet's own message, not just the provider's summary
				// and hint — both of those are provider-generated and would
				// match even if the server's response were discarded.
				ExpectError: regexp.MustCompile(`(?s)Error creating Android web app.*Android MDM isn't turned on\.`),
			},
		},
	})
}

// TestAccSoftwareWebAppResource_missingIconFile covers the local read failure
// path before any HTTP call is made.
func TestAccSoftwareWebAppResource_missingIconFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("provider must not call Fleet when the icon file cannot be read")
		http.NotFound(w, r)
	}))
	defer server.Close()

	cfg := fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_software_web_app" "test" {
  title     = "Missing Icon"
  url       = "https://example.com"
  icon_path = %[2]q
}
`, server.URL, filepath.Join(t.TempDir(), "does-not-exist.png"))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      cfg,
				ExpectError: regexp.MustCompile(`(?s)Unable to read icon file`),
			},
		},
	})
}

// testAccPreCheckAndroidMDM skips unless the live Fleet instance has Android
// MDM enabled and configured. POST /software/web_apps sits behind Fleet's
// VerifyAndroidMDM middleware, so without it the endpoint answers 400
// "Android MDM isn't turned on." — and Fleet's dev mode has no Android
// enterprise, so this skips on the CI rig.
//
// A transport or auth failure is a hard failure, not a skip: silently skipping
// on a bad token would make this test look green while covering nothing.
func testAccPreCheckAndroidMDM(t *testing.T) {
	t.Helper()

	// VerifyTLS must be set explicitly: ClientConfig's zero value turns into
	// InsecureSkipVerify, and a precheck has no reason to skip verification.
	client, err := fleetdm.NewClient(fleetdm.ClientConfig{
		ServerAddress: os.Getenv("FLEETDM_URL"),
		APIKey:        os.Getenv("FLEETDM_API_TOKEN"),
		VerifyTLS:     true,
	})
	if err != nil {
		t.Fatalf("could not build Fleet client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg, err := client.GetAppConfig(ctx)
	if err != nil {
		t.Fatalf("could not read Fleet app config to check Android MDM status: %v", err)
	}
	if cfg.MDM == nil || !cfg.MDM.AndroidEnabledAndConfigured {
		t.Skip("Android MDM is not enabled and configured on this Fleet instance; POST /software/web_apps is gated behind it")
	}
}

// TestAccSoftwareWebAppResource_live creates a real Android web app against a
// live Fleet instance. It skips unless Android MDM is enabled and configured,
// which Fleet's dev mode cannot do — so this does not run in CI today. There is
// no cleanup step because Fleet exposes no delete for web apps; the destroy at
// the end of the test only drops Terraform state.
func TestAccSoftwareWebAppResource_live(t *testing.T) {
	testAccPreCheck(t)
	testAccPreCheckAndroidMDM(t)

	suffix := acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	title := "tf-acc-webapp-" + suffix

	cfg := fmt.Sprintf(`
%[1]s

resource "fleetdm_software_web_app" "test" {
  title = %[2]q
  url   = "https://example.com/%[3]s"
}
`, providerConfig(), title, suffix)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_software_web_app.test", "title", title),
					resource.TestCheckResourceAttrSet("fleetdm_software_web_app.test", "app_store_id"),
					resource.TestMatchResourceAttr("fleetdm_software_web_app.test", "app_store_id",
						regexp.MustCompile(`^com\.google\.enterprise\.webapp\.`)),
				),
			},
		},
	})
}
