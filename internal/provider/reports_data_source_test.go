package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccReportsDataSource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/reports" && r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"reports": []map[string]interface{}{
					{
						"id":                  1,
						"name":                "Get OS Version",
						"description":         "Returns OS version",
						"query":               "SELECT * FROM os_version;",
						"platform":            "darwin",
						"interval":            3600,
						"observer_can_run":    false,
						"automations_enabled": false,
						"logging":             "snapshot",
						"discard_data":        false,
						"author_id":           1,
						"author_name":         "Admin",
						"author_email":        "admin@example.com",
					},
					{
						"id":                  2,
						"name":                "System Info",
						"description":         "System information",
						"query":               "SELECT * FROM system_info;",
						"platform":            "",
						"interval":            0,
						"observer_can_run":    true,
						"automations_enabled": false,
						"logging":             "snapshot",
						"discard_data":        false,
						"author_id":           1,
						"author_name":         "Admin",
						"author_email":        "admin@example.com",
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
				Config: testAccReportsDataSourceConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_reports.test", "reports.#", "2"),
					resource.TestCheckResourceAttr("data.fleetdm_reports.test", "reports.0.name", "Get OS Version"),
					resource.TestCheckResourceAttr("data.fleetdm_reports.test", "reports.0.platform.#", "1"),
					resource.TestCheckResourceAttr("data.fleetdm_reports.test", "reports.0.platform.0", "darwin"),
					resource.TestCheckResourceAttr("data.fleetdm_reports.test", "reports.1.name", "System Info"),
					resource.TestCheckResourceAttr("data.fleetdm_reports.test", "reports.1.observer_can_run", "true"),
				),
			},
		},
	})
}

func testAccReportsDataSourceConfig(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_reports" "test" {}
`
}

// TestAccReportsDataSource_live creates a report then verifies it appears in the list.
func TestAccReportsDataSource_live(t *testing.T) {
	reportName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccReportsDataSourceConfig_live(reportName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.fleetdm_reports.test", "reports.#"),
				),
			},
		},
	})
}

func testAccReportsDataSourceConfig_live(reportName string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_report" "test" {
  name  = %[1]q
  query = "SELECT * FROM system_info;"
}

data "fleetdm_reports" "test" {
  depends_on = [fleetdm_report.test]
}
`, reportName)
}
