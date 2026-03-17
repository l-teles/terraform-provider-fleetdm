package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
