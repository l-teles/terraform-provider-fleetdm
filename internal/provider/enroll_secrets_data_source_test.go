package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccEnrollSecretsDataSource_live reads global enroll secrets from a real Fleet instance.
func TestAccEnrollSecretsDataSource_live(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `data "fleetdm_enroll_secrets" "test" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.fleetdm_enroll_secrets.test", "secrets.#"),
				),
			},
		},
	})
}

func TestAccEnrollSecretsDataSource_global(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/spec/enroll_secret" && r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"spec": map[string]interface{}{
					"secrets": []map[string]interface{}{
						{"secret": "secret-abc123", "created_at": "2024-01-15T10:00:00Z"},
						{"secret": "secret-def456", "created_at": "2024-01-16T10:00:00Z"},
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
				Config: testAccEnrollSecretsDataSourceConfig_global(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_enroll_secrets.test", "secrets.#", "2"),
					resource.TestCheckResourceAttr("data.fleetdm_enroll_secrets.test", "secrets.0.secret", "secret-abc123"),
					resource.TestCheckResourceAttr("data.fleetdm_enroll_secrets.test", "secrets.1.secret", "secret-def456"),
				),
			},
		},
	})
}

func testAccEnrollSecretsDataSourceConfig_global(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_enroll_secrets" "test" {}
`
}
