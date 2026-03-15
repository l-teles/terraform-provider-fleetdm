package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSoftwareTitleDataSource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/software/titles/1" && r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"software_title": map[string]interface{}{
					"id":                1,
					"name":              "Google Chrome.app",
					"display_name":      "Google Chrome",
					"source":            "apps",
					"icon_url":          "",
					"hosts_count":       5,
					"versions_count":    1,
					"bundle_identifier": "com.google.Chrome",
					"versions": []map[string]interface{}{
						{
							"id":              1,
							"version":         "119.0.6045.105",
							"hosts_count":     5,
							"vulnerabilities": []interface{}{},
						},
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
				Config: testAccSoftwareTitleDataSourceConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_software_title.test", "name", "Google Chrome.app"),
					resource.TestCheckResourceAttr("data.fleetdm_software_title.test", "display_name", "Google Chrome"),
					resource.TestCheckResourceAttr("data.fleetdm_software_title.test", "source", "apps"),
					resource.TestCheckResourceAttr("data.fleetdm_software_title.test", "hosts_count", "5"),
					resource.TestCheckResourceAttr("data.fleetdm_software_title.test", "versions.#", "1"),
					resource.TestCheckResourceAttr("data.fleetdm_software_title.test", "versions.0.version", "119.0.6045.105"),
				),
			},
		},
	})
}

func testAccSoftwareTitleDataSourceConfig(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_software_title" "test" {
  id = 1
}
`
}
