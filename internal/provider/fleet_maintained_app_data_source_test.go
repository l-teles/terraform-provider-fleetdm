package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccFleetMaintainedAppDataSource_byName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/software/fleet_maintained_apps" && r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"fleet_maintained_apps": []map[string]interface{}{
					{
						"id":               1,
						"name":             "1Password",
						"slug":             "1password/darwin",
						"platform":         "darwin",
						"version":          "8.10.0",
						"filename":         "1password-8.10.0.pkg",
						"install_script":   "installer -pkg /tmp/1password.pkg -target /",
						"uninstall_script": "rm -rf /Applications/1Password.app",
					},
					{
						"id":               2,
						"name":             "Google Chrome",
						"slug":             "google-chrome/darwin",
						"platform":         "darwin",
						"version":          "120.0.0",
						"filename":         "google-chrome-120.0.0.pkg",
						"install_script":   "installer -pkg /tmp/chrome.pkg -target /",
						"uninstall_script": "rm -rf /Applications/Google Chrome.app",
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccFleetMaintainedAppDataSourceConfig_byName(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_fleet_maintained_app.test", "id", "1"),
					resource.TestCheckResourceAttr("data.fleetdm_fleet_maintained_app.test", "name", "1Password"),
					resource.TestCheckResourceAttr("data.fleetdm_fleet_maintained_app.test", "slug", "1password/darwin"),
					resource.TestCheckResourceAttr("data.fleetdm_fleet_maintained_app.test", "platform", "darwin"),
					resource.TestCheckResourceAttr("data.fleetdm_fleet_maintained_app.test", "version", "8.10.0"),
					resource.TestCheckResourceAttr("data.fleetdm_fleet_maintained_app.test", "filename", "1password-8.10.0.pkg"),
					resource.TestCheckResourceAttrSet("data.fleetdm_fleet_maintained_app.test", "install_script"),
					resource.TestCheckResourceAttrSet("data.fleetdm_fleet_maintained_app.test", "uninstall_script"),
				),
			},
		},
	})
}

func TestAccFleetMaintainedAppDataSource_byID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/software/fleet_maintained_apps/1" && r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"fleet_maintained_app": map[string]interface{}{
					"id":               1,
					"name":             "1Password",
					"slug":             "1password/darwin",
					"platform":         "darwin",
					"version":          "8.10.0",
					"filename":         "1password-8.10.0.pkg",
					"install_script":   "installer -pkg /tmp/1password.pkg -target /",
					"uninstall_script": "rm -rf /Applications/1Password.app",
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccFleetMaintainedAppDataSourceConfig_byID(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_fleet_maintained_app.test", "id", "1"),
					resource.TestCheckResourceAttr("data.fleetdm_fleet_maintained_app.test", "name", "1Password"),
					resource.TestCheckResourceAttr("data.fleetdm_fleet_maintained_app.test", "slug", "1password/darwin"),
					resource.TestCheckResourceAttr("data.fleetdm_fleet_maintained_app.test", "platform", "darwin"),
					resource.TestCheckResourceAttr("data.fleetdm_fleet_maintained_app.test", "version", "8.10.0"),
				),
			},
		},
	})
}

// newFleetMaintainedAppMultiPlatformServer serves a catalog where the same app
// name exists on two platforms, which is how Fleet publishes Firefox, Chrome,
// Slack and friends.
func newFleetMaintainedAppMultiPlatformServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/software/fleet_maintained_apps" && r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"fleet_maintained_apps": []map[string]interface{}{
					{
						"id":               11,
						"name":             "Mozilla Firefox",
						"slug":             "mozilla-firefox/darwin",
						"platform":         "darwin",
						"version":          "144.0.2",
						"filename":         "Firefox-144.0.2.dmg",
						"install_script":   "cp -R /tmp/Firefox.app /Applications/",
						"uninstall_script": "rm -rf /Applications/Firefox.app",
					},
					{
						"id":               93926,
						"name":             "Mozilla Firefox",
						"slug":             "mozilla-firefox/windows",
						"platform":         "windows",
						"version":          "144.0.2",
						"filename":         "Firefox-144.0.2.msi",
						"install_script":   "msiexec /i \"${env:INSTALLER_PATH}\" /quiet /norestart",
						"uninstall_script": "msiexec /x \"${env:UPGRADE_CODE}\" /quiet /norestart",
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
}

// TestAccFleetMaintainedAppDataSource_byNameAndPlatform pins the platform
// filter: without it a name lookup returns the first (darwin) match, which
// would hand the consuming resource the wrong platform's app ID.
func TestAccFleetMaintainedAppDataSource_byNameAndPlatform(t *testing.T) {
	server := newFleetMaintainedAppMultiPlatformServer()
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccFleetMaintainedAppDataSourceConfig_byNameAndPlatform(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_fleet_maintained_app.test", "id", "93926"),
					resource.TestCheckResourceAttr("data.fleetdm_fleet_maintained_app.test", "name", "Mozilla Firefox"),
					resource.TestCheckResourceAttr("data.fleetdm_fleet_maintained_app.test", "platform", "windows"),
					resource.TestCheckResourceAttr("data.fleetdm_fleet_maintained_app.test", "slug", "mozilla-firefox/windows"),
					resource.TestCheckResourceAttr("data.fleetdm_fleet_maintained_app.test", "filename", "Firefox-144.0.2.msi"),
				),
			},
		},
	})
}

// TestAccFleetMaintainedAppDataSource_platformNoMatch covers a name that exists
// but not on the requested platform.
func TestAccFleetMaintainedAppDataSource_platformNoMatch(t *testing.T) {
	server := newFleetMaintainedAppMultiPlatformServer()
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// \s+ for the spaces: Terraform hard-wraps diagnostic text.
				Config:      testAccFleetMaintainedAppDataSourceConfig_platformNoMatch(server.URL),
				ExpectError: regexp.MustCompile(`(?s)No\s+Fleet\s+Maintained\s+App\s+found\s+with\s+name\s+"Mozilla\s+Firefox"\s+and\s+platform\s+"linux"`),
			},
		},
	})
}

// TestAccFleetMaintainedAppDataSource_invalidPlatform checks the schema
// rejects "macos", the common wrong guess for "darwin".
func TestAccFleetMaintainedAppDataSource_invalidPlatform(t *testing.T) {
	server := newFleetMaintainedAppMultiPlatformServer()
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccFleetMaintainedAppDataSourceConfig_invalidPlatform(server.URL),
				ExpectError: regexp.MustCompile(`(?i)value must be one of`),
			},
		},
	})
}

func testAccFleetMaintainedAppDataSourceConfig_byName(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_fleet_maintained_app" "test" {
  name = "1Password"
}
`
}

// newFleetMaintainedAppByIDServer serves a single darwin app at id 1, for
// tests that pair id with a name/platform the resolved app doesn't have.
func newFleetMaintainedAppByIDServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/software/fleet_maintained_apps/1" && r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"fleet_maintained_app": map[string]interface{}{
					"id":               1,
					"name":             "1Password",
					"slug":             "1password/darwin",
					"platform":         "darwin",
					"version":          "8.10.0",
					"filename":         "1password-8.10.0.pkg",
					"install_script":   "installer -pkg /tmp/1password.pkg -target /",
					"uninstall_script": "rm -rf /Applications/1Password.app",
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
}

// TestAccFleetMaintainedAppDataSource_byIDPlatformMismatch covers id=1
// (darwin) configured alongside a platform the resolved app doesn't have:
// this must error rather than silently overwrite platform in state.
func TestAccFleetMaintainedAppDataSource_byIDPlatformMismatch(t *testing.T) {
	server := newFleetMaintainedAppByIDServer()
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccFleetMaintainedAppDataSourceConfig_byIDPlatformMismatch(server.URL),
				ExpectError: regexp.MustCompile(`(?s)id\s+1\s+resolves\s+to\s+platform\s+"darwin",\s+but\s+the\s+configuration\s+specifies\s+platform\s+"windows"`),
			},
		},
	})
}

// TestAccFleetMaintainedAppDataSource_byIDNameMismatch mirrors the platform
// case for name, the pre-existing counterpart of the same bug.
func TestAccFleetMaintainedAppDataSource_byIDNameMismatch(t *testing.T) {
	server := newFleetMaintainedAppByIDServer()
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccFleetMaintainedAppDataSourceConfig_byIDNameMismatch(server.URL),
				ExpectError: regexp.MustCompile(`(?s)id\s+1\s+resolves\s+to\s+name\s+"1Password",\s+but\s+the\s+configuration\s+specifies\s+name\s+"Google\s+Chrome"`),
			},
		},
	})
}

func testAccFleetMaintainedAppDataSourceConfig_byIDPlatformMismatch(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_fleet_maintained_app" "test" {
  id       = 1
  platform = "windows"
}
`
}

func testAccFleetMaintainedAppDataSourceConfig_byIDNameMismatch(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_fleet_maintained_app" "test" {
  id   = 1
  name = "Google Chrome"
}
`
}

func testAccFleetMaintainedAppDataSourceConfig_byID(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_fleet_maintained_app" "test" {
  id = 1
}
`
}

func testAccFleetMaintainedAppDataSourceConfig_byNameAndPlatform(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_fleet_maintained_app" "test" {
  name     = "Mozilla Firefox"
  platform = "windows"
}
`
}

func testAccFleetMaintainedAppDataSourceConfig_platformNoMatch(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_fleet_maintained_app" "test" {
  name     = "Mozilla Firefox"
  platform = "linux"
}
`
}

func testAccFleetMaintainedAppDataSourceConfig_invalidPlatform(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_fleet_maintained_app" "test" {
  name     = "Mozilla Firefox"
  platform = "macos"
}
`
}

// TestAccFleetMaintainedAppDataSource_ambiguousName covers a name matching
// more than one platform with no platform set to narrow it: this must error
// rather than silently resolving to whichever entry the API lists first.
func TestAccFleetMaintainedAppDataSource_ambiguousName(t *testing.T) {
	server := newFleetMaintainedAppMultiPlatformServer()
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccFleetMaintainedAppDataSourceConfig_ambiguousName(server.URL),
				ExpectError: regexp.MustCompile(`(?s)2\s+Fleet\s+Maintained\s+Apps\s+found\s+with\s+name\s+"Mozilla\s+Firefox",\s+on\s+platforms\s+darwin,\s+windows\.\s+Set\s+platform\s+to\s+disambiguate`),
			},
		},
	})
}

func testAccFleetMaintainedAppDataSourceConfig_ambiguousName(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_fleet_maintained_app" "test" {
  name = "Mozilla Firefox"
}
`
}

// TestAccFleetMaintainedAppDataSource_byIDWithTeamID covers that team_id
// reaches the by-id lookup (it used to be silently dropped) and populates
// software_title_id for that team.
func TestAccFleetMaintainedAppDataSource_byIDWithTeamID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/software/fleet_maintained_apps/1" && r.Method == "GET" {
			body := map[string]interface{}{
				"id":       1,
				"name":     "1Password",
				"slug":     "1password/darwin",
				"platform": "darwin",
				"version":  "8.10.0",
			}
			if r.URL.Query().Get("team_id") == "5" {
				body["software_title_id"] = 42
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"fleet_maintained_app": body})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccFleetMaintainedAppDataSourceConfig_byIDWithTeamID(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_fleet_maintained_app.test", "id", "1"),
					resource.TestCheckResourceAttr("data.fleetdm_fleet_maintained_app.test", "software_title_id", "42"),
				),
			},
		},
	})
}

func testAccFleetMaintainedAppDataSourceConfig_byIDWithTeamID(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_fleet_maintained_app" "test" {
  id      = 1
  team_id = 5
}
`
}
