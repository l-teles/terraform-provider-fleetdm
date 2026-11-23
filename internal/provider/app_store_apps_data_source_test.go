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

// TestAccAppStoreAppsDataSource_vppTokenID checks vpp_token_id reaches Fleet
// as a query parameter, which Fleet 4.95 needs for a team with several tokens.
func TestAccAppStoreAppsDataSource_vppTokenID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/software/app_store_apps" && r.Method == "GET" {
			q := r.URL.Query()
			if q.Get("team_id") != "5" || q.Get("vpp_token_id") != "3" {
				http.Error(w, `{"message":"This fleet has more than one VPP token. Please specify vpp_token_id."}`, http.StatusUnprocessableEntity)
				return
			}
			_, _ = w.Write([]byte(`{"app_store_apps":[{"app_store_id":"1","name":"Example App","platform":"ios"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "fleetdm" {
  server_address = "` + server.URL + `"
  api_key        = "test-token"
}

data "fleetdm_app_store_apps" "test" {
  team_id      = 5
  vpp_token_id = 3
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_app_store_apps.test", "vpp_token_id", "3"),
					resource.TestCheckResourceAttr("data.fleetdm_app_store_apps.test", "app_store_apps.#", "1"),
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
