package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccAppStoreAppsDataSource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/software/app_store_apps" && r.Method == "GET" {
			if r.URL.Query().Get("team_id") != "5" {
				http.Error(w, "missing team_id", http.StatusBadRequest)
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"app_store_apps": []map[string]interface{}{
					{
						"app_store_id":   "361309726",
						"name":           "TestFlight",
						"platform":       "darwin",
						"icon_url":       "https://example.com/testflight.png",
						"latest_version": "3.2.0",
					},
					{
						"app_store_id":   "497799835",
						"name":           "Xcode",
						"platform":       "darwin",
						"icon_url":       "https://example.com/xcode.png",
						"latest_version": "15.2",
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
				Config: testAccAppStoreAppsDataSourceConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_app_store_apps.test", "team_id", "5"),
					resource.TestCheckResourceAttr("data.fleetdm_app_store_apps.test", "app_store_apps.#", "2"),
					resource.TestCheckResourceAttr("data.fleetdm_app_store_apps.test", "app_store_apps.0.app_store_id", "361309726"),
					resource.TestCheckResourceAttr("data.fleetdm_app_store_apps.test", "app_store_apps.0.name", "TestFlight"),
					resource.TestCheckResourceAttr("data.fleetdm_app_store_apps.test", "app_store_apps.0.platform", "darwin"),
					resource.TestCheckResourceAttr("data.fleetdm_app_store_apps.test", "app_store_apps.0.icon_url", "https://example.com/testflight.png"),
					resource.TestCheckResourceAttr("data.fleetdm_app_store_apps.test", "app_store_apps.0.latest_version", "3.2.0"),
					resource.TestCheckResourceAttr("data.fleetdm_app_store_apps.test", "app_store_apps.1.app_store_id", "497799835"),
					resource.TestCheckResourceAttr("data.fleetdm_app_store_apps.test", "app_store_apps.1.name", "Xcode"),
				),
			},
		},
	})
}

func testAccAppStoreAppsDataSourceConfig(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_app_store_apps" "test" {
  team_id = 5
}
`
}
