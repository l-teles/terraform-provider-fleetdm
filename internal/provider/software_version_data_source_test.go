package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSoftwareVersionDataSource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/software/versions/10" && r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"software": map[string]interface{}{
					"id":                10,
					"name":              "Slack.app",
					"version":           "4.35.126",
					"source":            "apps",
					"bundle_identifier": "com.tinyspeck.slackmacgap",
					"hosts_count":       3,
					"vulnerabilities":   []interface{}{},
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
				Config: testAccSoftwareVersionDataSourceConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_software_version.test", "id", "10"),
					resource.TestCheckResourceAttr("data.fleetdm_software_version.test", "name", "Slack.app"),
					resource.TestCheckResourceAttr("data.fleetdm_software_version.test", "version", "4.35.126"),
					resource.TestCheckResourceAttr("data.fleetdm_software_version.test", "source", "apps"),
					resource.TestCheckResourceAttr("data.fleetdm_software_version.test", "hosts_count", "3"),
				),
			},
		},
	})
}

func testAccSoftwareVersionDataSourceConfig(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_software_version" "test" {
  id = 10
}
`
}
