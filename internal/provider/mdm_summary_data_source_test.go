package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccMDMSummaryDataSource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/hosts/summary/mdm" && r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"counts_updated_at": "2024-01-15T10:00:00Z",
				"mobile_device_management_enrollment_status": map[string]interface{}{
					"enrolled_manual_hosts_count":    5,
					"enrolled_automated_hosts_count": 10,
					"enrolled_personal_hosts_count":  2,
					"unenrolled_hosts_count":         3,
					"pending_hosts_count":            1,
					"hosts_count":                    21,
				},
				"mobile_device_management_solution": []map[string]interface{}{
					{
						"id":          1,
						"name":        "Fleet",
						"server_url":  "https://fleet.example.com/mdm/apple/mdm",
						"hosts_count": 17,
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
				Config: testAccMDMSummaryDataSourceConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_mdm_summary.test", "enrolled_manual_hosts_count", "5"),
					resource.TestCheckResourceAttr("data.fleetdm_mdm_summary.test", "enrolled_automated_hosts_count", "10"),
					resource.TestCheckResourceAttr("data.fleetdm_mdm_summary.test", "unenrolled_hosts_count", "3"),
					resource.TestCheckResourceAttr("data.fleetdm_mdm_summary.test", "hosts_count", "21"),
					resource.TestCheckResourceAttr("data.fleetdm_mdm_summary.test", "mdm_solutions.#", "1"),
					resource.TestCheckResourceAttr("data.fleetdm_mdm_summary.test", "mdm_solutions.0.name", "Fleet"),
					resource.TestCheckResourceAttr("data.fleetdm_mdm_summary.test", "mdm_solutions.0.hosts_count", "17"),
				),
			},
		},
	})
}

func testAccMDMSummaryDataSourceConfig(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_mdm_summary" "test" {}
`
}
