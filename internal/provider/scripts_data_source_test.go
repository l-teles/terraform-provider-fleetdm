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

func TestAccScriptsDataSource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/scripts" && r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"scripts": []map[string]interface{}{
					{
						"id":         1,
						"name":       "update-system.sh",
						"created_at": "2024-01-15T10:00:00Z",
						"updated_at": "2024-01-15T10:00:00Z",
					},
					{
						"id":         2,
						"name":       "collect-logs.sh",
						"created_at": "2024-01-16T10:00:00Z",
						"updated_at": "2024-01-16T10:00:00Z",
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
				Config: testAccScriptsDataSourceConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_scripts.test", "scripts.#", "2"),
					resource.TestCheckResourceAttr("data.fleetdm_scripts.test", "scripts.0.name", "update-system.sh"),
					resource.TestCheckResourceAttr("data.fleetdm_scripts.test", "scripts.1.name", "collect-logs.sh"),
				),
			},
		},
	})
}

func testAccScriptsDataSourceConfig(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_scripts" "test" {}
`
}

// TestAccScriptsDataSource_live creates a script then verifies it appears in the list.
func TestAccScriptsDataSource_live(t *testing.T) {
	scriptName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum) + ".sh"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccScriptsDataSourceConfig_live(scriptName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.fleetdm_scripts.test", "scripts.#"),
				),
			},
		},
	})
}

func testAccScriptsDataSourceConfig_live(scriptName string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_script" "test" {
  name    = %[1]q
  content = "#!/bin/bash\necho 'hello'"
}

data "fleetdm_scripts" "test" {
  depends_on = [fleetdm_script.test]
}
`, scriptName)
}
