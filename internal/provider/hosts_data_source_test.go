package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccHostsDataSource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/hosts" && r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"hosts": []map[string]interface{}{
					{
						"id":              1,
						"uuid":            "abc-123",
						"hostname":        "macbook-pro.local",
						"display_name":    "MacBook Pro",
						"platform":        "darwin",
						"os_version":      "macOS 14.0",
						"hardware_vendor": "Apple",
						"hardware_model":  "MacBookPro18,1",
						"hardware_serial": "SN12345",
						"primary_ip":      "192.168.1.100",
						"team_id":         nil,
						"team_name":       "",
						"status":          "online",
					},
					{
						"id":              2,
						"uuid":            "def-456",
						"hostname":        "windows-server.local",
						"display_name":    "Windows Server",
						"platform":        "windows",
						"os_version":      "Windows Server 2022",
						"hardware_vendor": "Dell",
						"hardware_model":  "PowerEdge R740",
						"hardware_serial": "SN67890",
						"primary_ip":      "192.168.1.101",
						"team_id":         nil,
						"team_name":       "",
						"status":          "offline",
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
				Config: testAccHostsDataSourceConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_hosts.test", "hosts.#", "2"),
					resource.TestCheckResourceAttr("data.fleetdm_hosts.test", "hosts.0.hostname", "macbook-pro.local"),
					resource.TestCheckResourceAttr("data.fleetdm_hosts.test", "hosts.0.platform", "darwin"),
					resource.TestCheckResourceAttr("data.fleetdm_hosts.test", "hosts.0.status", "online"),
					resource.TestCheckResourceAttr("data.fleetdm_hosts.test", "hosts.1.hostname", "windows-server.local"),
					resource.TestCheckResourceAttr("data.fleetdm_hosts.test", "hosts.1.platform", "windows"),
					resource.TestCheckResourceAttr("data.fleetdm_hosts.test", "hosts.1.status", "offline"),
				),
			},
		},
	})
}

func testAccHostsDataSourceConfig(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_hosts" "test" {}
`
}
