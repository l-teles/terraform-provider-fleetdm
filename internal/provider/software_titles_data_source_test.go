package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSoftwareTitlesDataSource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/software/titles" && r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"software_titles": []map[string]interface{}{
					{
						"id":                1,
						"name":              "Google Chrome.app",
						"display_name":      "Google Chrome",
						"source":            "apps",
						"icon_url":          "",
						"hosts_count":       5,
						"versions_count":    2,
						"bundle_identifier": "com.google.Chrome",
					},
					{
						"id":                2,
						"name":              "Slack.app",
						"display_name":      "Slack",
						"source":            "apps",
						"icon_url":          "",
						"hosts_count":       3,
						"versions_count":    1,
						"bundle_identifier": "com.tinyspeck.slackmacgap",
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
				Config: testAccSoftwareTitlesDataSourceConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_software_titles.test", "software_titles.#", "2"),
					resource.TestCheckResourceAttr("data.fleetdm_software_titles.test", "software_titles.0.name", "Google Chrome.app"),
					resource.TestCheckResourceAttr("data.fleetdm_software_titles.test", "software_titles.0.hosts_count", "5"),
					resource.TestCheckResourceAttr("data.fleetdm_software_titles.test", "software_titles.1.name", "Slack.app"),
					resource.TestCheckResourceAttr("data.fleetdm_software_titles.test", "total_count", "2"),
				),
			},
		},
	})
}

func testAccSoftwareTitlesDataSourceConfig(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_software_titles" "test" {}
`
}
