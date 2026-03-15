package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccABMTokensDataSource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/abm_tokens" && r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"abm_tokens": []map[string]interface{}{
					{
						"id":               1,
						"apple_id":         "admin@example.com",
						"org_name":         "Example Corp",
						"mdm_server_url":   "https://fleet.example.com/mdm/apple/mdm",
						"renew_date":       "2025-12-31T00:00:00Z",
						"terms_expired":    false,
						"macos_team_id":    nil,
						"ios_team_id":      nil,
						"ipados_team_id":   nil,
						"macos_team_name":  "",
						"ios_team_name":    "",
						"ipados_team_name": "",
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
				Config: testAccABMTokensDataSourceConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_abm_tokens.test", "tokens.#", "1"),
					resource.TestCheckResourceAttr("data.fleetdm_abm_tokens.test", "tokens.0.id", "1"),
					resource.TestCheckResourceAttr("data.fleetdm_abm_tokens.test", "tokens.0.apple_id", "admin@example.com"),
					resource.TestCheckResourceAttr("data.fleetdm_abm_tokens.test", "tokens.0.organization_name", "Example Corp"),
					resource.TestCheckResourceAttr("data.fleetdm_abm_tokens.test", "tokens.0.terms_expired", "false"),
				),
			},
		},
	})
}

func testAccABMTokensDataSourceConfig(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_abm_tokens" "test" {}
`
}
