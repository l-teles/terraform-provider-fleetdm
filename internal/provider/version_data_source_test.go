package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccVersionDataSource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/version" && r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"version":    "1.2.3",
				"branch":     "main",
				"revision":   "abc123",
				"go_version": "go1.21.0",
				"build_date": "2024-01-01",
				"build_user": "builder",
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
				Config: testAccVersionDataSourceConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_version.test", "version", "1.2.3"),
					resource.TestCheckResourceAttr("data.fleetdm_version.test", "branch", "main"),
					resource.TestCheckResourceAttr("data.fleetdm_version.test", "revision", "abc123"),
					resource.TestCheckResourceAttr("data.fleetdm_version.test", "go_version", "go1.21.0"),
					resource.TestCheckResourceAttr("data.fleetdm_version.test", "build_date", "2024-01-01"),
					resource.TestCheckResourceAttr("data.fleetdm_version.test", "build_user", "builder"),
				),
			},
		},
	})
}

func testAccVersionDataSourceConfig(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_version" "test" {}
`
}

// TestAccVersionDataSource_live tests the version data source against a real Fleet instance.
func TestAccVersionDataSource_live(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `data "fleetdm_version" "test" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.fleetdm_version.test", "version"),
					resource.TestCheckResourceAttrSet("data.fleetdm_version.test", "go_version"),
				),
			},
		},
	})
}
