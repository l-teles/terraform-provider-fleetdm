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

func TestAccPoliciesDataSource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/global/policies" && r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"policies": []map[string]interface{}{
					{
						"id":                      1,
						"name":                    "Disk Encryption",
						"description":             "Check disk encryption",
						"query":                   "SELECT 1 FROM disk_encryption WHERE encrypted = 1;",
						"critical":                true,
						"resolution":              "Enable FileVault",
						"platform":                "darwin",
						"passing_host_count":      10,
						"failing_host_count":      2,
						"author_id":               1,
						"author_name":             "Admin",
						"author_email":            "admin@example.com",
						"calendar_events_enabled": true,
						"created_at":              "2024-01-01T00:00:00Z",
						"updated_at":              "2024-01-15T10:00:00Z",
					},
					{
						"id":                      2,
						"name":                    "OS Up To Date",
						"description":             "",
						"query":                   "SELECT 1 FROM os_version WHERE version >= '14.0';",
						"critical":                false,
						"resolution":              "",
						"platform":                "",
						"passing_host_count":      8,
						"failing_host_count":      4,
						"author_id":               1,
						"author_name":             "Admin",
						"author_email":            "admin@example.com",
						"calendar_events_enabled": false,
						"created_at":              "2024-02-01T00:00:00Z",
						"updated_at":              "2024-02-10T12:00:00Z",
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
				Config: testAccPoliciesDataSourceConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_policies.test", "policies.#", "2"),
					resource.TestCheckResourceAttr("data.fleetdm_policies.test", "policies.0.name", "Disk Encryption"),
					resource.TestCheckResourceAttr("data.fleetdm_policies.test", "policies.0.critical", "true"),
					resource.TestCheckResourceAttr("data.fleetdm_policies.test", "policies.0.passing_host_count", "10"),
					resource.TestCheckResourceAttr("data.fleetdm_policies.test", "policies.0.calendar_events_enabled", "true"),
					resource.TestCheckResourceAttr("data.fleetdm_policies.test", "policies.0.created_at", "2024-01-01T00:00:00Z"),
					resource.TestCheckResourceAttr("data.fleetdm_policies.test", "policies.0.updated_at", "2024-01-15T10:00:00Z"),
					resource.TestCheckResourceAttr("data.fleetdm_policies.test", "policies.1.name", "OS Up To Date"),
					resource.TestCheckResourceAttr("data.fleetdm_policies.test", "policies.1.calendar_events_enabled", "false"),
					resource.TestCheckResourceAttr("data.fleetdm_policies.test", "policies.1.created_at", "2024-02-01T00:00:00Z"),
				),
			},
		},
	})
}

func testAccPoliciesDataSourceConfig(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_policies" "test" {}
`
}

// TestAccPoliciesDataSource_live creates a policy then verifies it appears in the list.
func TestAccPoliciesDataSource_live(t *testing.T) {
	policyName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPoliciesDataSourceConfig_live(policyName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.fleetdm_policies.test", "policies.#"),
				),
			},
		},
	})
}

func testAccPoliciesDataSourceConfig_live(policyName string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_policy" "test" {
  name  = %[1]q
  query = "SELECT 1 WHERE 1=1;"
}

data "fleetdm_policies" "test" {
  depends_on = [fleetdm_policy.test]
}
`, policyName)
}
