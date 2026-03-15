package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSoftwareVersionsDataSource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/software/versions" && r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"software": []map[string]interface{}{
					{
						"id":                1,
						"name":              "Google Chrome.app",
						"version":           "119.0.6045.105",
						"source":            "apps",
						"bundle_identifier": "com.google.Chrome",
						"hosts_count":       5,
						"vulnerabilities":   []interface{}{},
					},
					{
						"id":                2,
						"name":              "Slack.app",
						"version":           "4.35.126",
						"source":            "apps",
						"bundle_identifier": "com.tinyspeck.slackmacgap",
						"hosts_count":       3,
						"vulnerabilities":   []interface{}{},
					},
				},
				"count": 2,
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
				Config: testAccSoftwareVersionsDataSourceConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_software_versions.test", "software_versions.#", "2"),
					resource.TestCheckResourceAttr("data.fleetdm_software_versions.test", "software_versions.0.name", "Google Chrome.app"),
					resource.TestCheckResourceAttr("data.fleetdm_software_versions.test", "software_versions.0.version", "119.0.6045.105"),
					resource.TestCheckResourceAttr("data.fleetdm_software_versions.test", "software_versions.1.name", "Slack.app"),
					resource.TestCheckResourceAttr("data.fleetdm_software_versions.test", "total_count", "2"),
				),
			},
		},
	})
}

func testAccSoftwareVersionsDataSourceConfig(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_software_versions" "test" {}
`
}
