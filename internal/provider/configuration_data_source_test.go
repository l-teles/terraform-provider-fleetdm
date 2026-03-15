package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccConfigurationDataSource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/config" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"org_info": map[string]interface{}{
					"org_name":     "Test Org",
					"org_logo_url": "",
					"contact_url":  "https://fleetdm.com/company/contact",
				},
				"server_settings": map[string]interface{}{
					"server_url":             "https://fleet.example.com",
					"live_query_disabled":    false,
					"enable_analytics":       true,
					"query_reports_disabled": false,
					"scripts_disabled":       false,
				},
				"host_expiry_settings": map[string]interface{}{
					"host_expiry_enabled": true,
					"host_expiry_window":  30,
				},
				"activity_expiry_settings": map[string]interface{}{
					"activity_expiry_enabled": false,
					"activity_expiry_window":  0,
				},
				"features": map[string]interface{}{
					"enable_host_users":         true,
					"enable_software_inventory": true,
				},
				"fleet_desktop": map[string]interface{}{
					"transparency_url": "https://fleetdm.com/transparency",
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
				Config: testAccConfigurationDataSourceConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_configuration.test", "org_name", "Test Org"),
					resource.TestCheckResourceAttr("data.fleetdm_configuration.test", "host_expiry_enabled", "true"),
					resource.TestCheckResourceAttr("data.fleetdm_configuration.test", "host_expiry_window", "30"),
					resource.TestCheckResourceAttr("data.fleetdm_configuration.test", "enable_host_users", "true"),
					resource.TestCheckResourceAttr("data.fleetdm_configuration.test", "enable_software_inventory", "true"),
				),
			},
		},
	})
}

func testAccConfigurationDataSourceConfig(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_configuration" "test" {}
`
}

// TestAccConfigurationDataSource_live tests the configuration data source against a real Fleet instance.
func TestAccConfigurationDataSource_live(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `data "fleetdm_configuration" "test" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.fleetdm_configuration.test", "org_name"),
					resource.TestCheckResourceAttrSet("data.fleetdm_configuration.test", "server_url"),
				),
			},
		},
	})
}
