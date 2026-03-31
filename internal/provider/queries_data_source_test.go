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

func TestAccQueriesDataSource_basic(t *testing.T) {
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
				Config: testAccQueriesDataSourceConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_queries.test", "queries.#", "2"),
					resource.TestCheckResourceAttr("data.fleetdm_queries.test", "queries.0.name", "Get OS Version"),
					resource.TestCheckResourceAttr("data.fleetdm_queries.test", "queries.0.platform.#", "1"),
					resource.TestCheckResourceAttr("data.fleetdm_queries.test", "queries.0.platform.0", "darwin"),
					resource.TestCheckResourceAttr("data.fleetdm_queries.test", "queries.1.name", "System Info"),
					resource.TestCheckResourceAttr("data.fleetdm_queries.test", "queries.1.observer_can_run", "true"),
				),
			},
		},
	})
}

func testAccQueriesDataSourceConfig(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_queries" "test" {}
`
}

// TestAccQueriesDataSource_live creates a query then verifies it appears in the list.
func TestAccQueriesDataSource_live(t *testing.T) {
	queryName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccQueriesDataSourceConfig_live(queryName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.fleetdm_queries.test", "queries.#"),
				),
			},
		},
	})
}

func testAccQueriesDataSourceConfig_live(queryName string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_query" "test" {
  name  = %[1]q
  query = "SELECT * FROM system_info;"
}

data "fleetdm_queries" "test" {
  depends_on = [fleetdm_query.test]
}
`, queryName)
}
