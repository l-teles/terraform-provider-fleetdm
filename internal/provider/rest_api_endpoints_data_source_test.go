package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccRESTAPIEndpointsDataSource_basic pins the mapping of Fleet's
// GET /rest_api catalog onto the data source, including the `:name` path
// placeholders and the deprecated flag.
func TestAccRESTAPIEndpointsDataSource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/fleet/rest_api" && r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"api_endpoints":[
				{"method":"GET","path":"/api/v1/fleet/hosts","display_name":"List hosts","deprecated":false},
				{"method":"GET","path":"/api/v1/fleet/hosts/:id","display_name":"Get host","deprecated":false},
				{"method":"GET","path":"/api/v1/fleet/mdm/apple/profiles","display_name":"List custom macOS settings (configuration profiles)","deprecated":true}
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
				Config: testAccRESTAPIEndpointsDataSourceConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_rest_api_endpoints.test", "api_endpoints.#", "3"),
					resource.TestCheckResourceAttr("data.fleetdm_rest_api_endpoints.test", "api_endpoints.0.method", "GET"),
					resource.TestCheckResourceAttr("data.fleetdm_rest_api_endpoints.test", "api_endpoints.0.path", "/api/v1/fleet/hosts"),
					resource.TestCheckResourceAttr("data.fleetdm_rest_api_endpoints.test", "api_endpoints.0.display_name", "List hosts"),
					resource.TestCheckResourceAttr("data.fleetdm_rest_api_endpoints.test", "api_endpoints.0.deprecated", "false"),
					// Route templates must survive verbatim: Fleet matches an
					// api_endpoints scope against the exact template.
					resource.TestCheckResourceAttr("data.fleetdm_rest_api_endpoints.test", "api_endpoints.1.path", "/api/v1/fleet/hosts/:id"),
					resource.TestCheckResourceAttr("data.fleetdm_rest_api_endpoints.test", "api_endpoints.2.deprecated", "true"),
				),
			},
		},
	})
}

// TestAccRESTAPIEndpointsDataSource_empty covers a catalog with no entries, so
// the data source yields an empty list rather than a null.
func TestAccRESTAPIEndpointsDataSource_empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"api_endpoints":[]}`))
	}))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccRESTAPIEndpointsDataSourceConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_rest_api_endpoints.test", "api_endpoints.#", "0"),
				),
			},
		},
	})
}

// TestAccRESTAPIEndpointsDataSource_live reads the catalog from a real Fleet
// instance. The endpoint is Premium-only, so this asserts on shape rather than
// on any specific entry.
func TestAccRESTAPIEndpointsDataSource_live(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `data "fleetdm_rest_api_endpoints" "test" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.fleetdm_rest_api_endpoints.test", "api_endpoints.#"),
					resource.TestCheckResourceAttrSet("data.fleetdm_rest_api_endpoints.test", "api_endpoints.0.method"),
					resource.TestCheckResourceAttrSet("data.fleetdm_rest_api_endpoints.test", "api_endpoints.0.path"),
					resource.TestCheckResourceAttrSet("data.fleetdm_rest_api_endpoints.test", "api_endpoints.0.display_name"),
				),
			},
		},
	})
}

func testAccRESTAPIEndpointsDataSourceConfig(serverURL string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-key"
}

data "fleetdm_rest_api_endpoints" "test" {}
`, serverURL)
}
