package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccVPPTokensDataSource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/vpp_tokens" && r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"vpp_tokens": []map[string]interface{}{
					{
						"id":         1,
						"org_name":   "Example Corp",
						"location":   "Main Office",
						"renew_date": "2025-12-31T00:00:00Z",
						"teams":      []interface{}{},
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
				Config: testAccVPPTokensDataSourceConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_vpp_tokens.test", "tokens.#", "1"),
					resource.TestCheckResourceAttr("data.fleetdm_vpp_tokens.test", "tokens.0.id", "1"),
					resource.TestCheckResourceAttr("data.fleetdm_vpp_tokens.test", "tokens.0.organization_name", "Example Corp"),
					resource.TestCheckResourceAttr("data.fleetdm_vpp_tokens.test", "tokens.0.location", "Main Office"),
					resource.TestCheckResourceAttr("data.fleetdm_vpp_tokens.test", "tokens.0.teams.#", "0"),
				),
			},
		},
	})
}

func testAccVPPTokensDataSourceConfig(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_vpp_tokens" "test" {}
`
}
