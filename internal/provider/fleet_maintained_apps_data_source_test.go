package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccFleetMaintainedAppsDataSource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/software/fleet_maintained_apps" && r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"fleet_maintained_apps": []map[string]interface{}{
					{
						"id":       1,
						"name":     "1Password",
						"slug":     "1password/darwin",
						"platform": "darwin",
						"version":  "8.10.0",
					},
					{
						"id":       2,
						"name":     "Google Chrome",
						"slug":     "google-chrome/darwin",
						"platform": "darwin",
						"version":  "120.0.0",
					},
					{
						"id":                3,
						"name":              "Slack",
						"slug":              "slack/darwin",
						"platform":          "darwin",
						"version":           "4.38.0",
						"software_title_id": 42,
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
				Config: testAccFleetMaintainedAppsDataSourceConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_fleet_maintained_apps.test", "fleet_maintained_apps.#", "3"),
					resource.TestCheckResourceAttr("data.fleetdm_fleet_maintained_apps.test", "fleet_maintained_apps.0.name", "1Password"),
					resource.TestCheckResourceAttr("data.fleetdm_fleet_maintained_apps.test", "fleet_maintained_apps.0.platform", "darwin"),
					resource.TestCheckResourceAttr("data.fleetdm_fleet_maintained_apps.test", "fleet_maintained_apps.1.name", "Google Chrome"),
					resource.TestCheckResourceAttr("data.fleetdm_fleet_maintained_apps.test", "fleet_maintained_apps.2.name", "Slack"),
					resource.TestCheckResourceAttr("data.fleetdm_fleet_maintained_apps.test", "fleet_maintained_apps.2.software_title_id", "42"),
				),
			},
		},
	})
}

func testAccFleetMaintainedAppsDataSourceConfig(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_fleet_maintained_apps" "test" {}
`
}
