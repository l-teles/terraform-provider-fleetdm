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

// TestAccVPPTokensDataSource_495Shape covers the Fleet 4.95 token shape: team
// tuples keyed `team_id`, `country_code`, `created_at`, and the null (no team)
// vs empty (all teams) distinction.
func TestAccVPPTokensDataSource_495Shape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/vpp_tokens" && r.Method == "GET" {
			_, _ = w.Write([]byte(`{"vpp_tokens":[
				{"id":3,"org_name":"Example Org","location":"Example Unit","renew_date":"2030-01-01T00:00:00Z",
				 "country_code":"us","created_at":"2026-01-01T00:00:00Z",
				 "teams":[{"team_id":7,"name":"Workstations"}],"fleets":[{"fleet_id":7,"name":"Workstations"}]},
				{"id":4,"org_name":"Example Org","location":"Other Unit","renew_date":"2030-01-01T00:00:00Z","teams":null}
			]}`))
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
					resource.TestCheckResourceAttr("data.fleetdm_vpp_tokens.test", "tokens.#", "2"),
					resource.TestCheckResourceAttr("data.fleetdm_vpp_tokens.test", "tokens.0.country_code", "us"),
					resource.TestCheckResourceAttr("data.fleetdm_vpp_tokens.test", "tokens.0.created_at", "2026-01-01T00:00:00Z"),
					resource.TestCheckResourceAttr("data.fleetdm_vpp_tokens.test", "tokens.0.teams.#", "1"),
					resource.TestCheckResourceAttr("data.fleetdm_vpp_tokens.test", "tokens.0.teams.0.id", "7"),
					resource.TestCheckResourceAttr("data.fleetdm_vpp_tokens.test", "tokens.0.teams.0.name", "Workstations"),
					resource.TestCheckNoResourceAttr("data.fleetdm_vpp_tokens.test", "tokens.1.country_code"),
					resource.TestCheckNoResourceAttr("data.fleetdm_vpp_tokens.test", "tokens.1.created_at"),
					resource.TestCheckNoResourceAttr("data.fleetdm_vpp_tokens.test", "tokens.1.teams.#"),
				),
			},
		},
	})
}
