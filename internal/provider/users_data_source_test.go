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

func TestAccUsersDataSource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/fleet/users" && r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"users": []map[string]interface{}{
					{
						"id":          1,
						"name":        "Alice Admin",
						"email":       "alice@example.com",
						"global_role": "admin",
						"sso_enabled": false,
						"api_only":    false,
						"teams":       []interface{}{},
					},
					{
						"id":          2,
						"name":        "Bob Observer",
						"email":       "bob@example.com",
						"global_role": "observer",
						"sso_enabled": false,
						"api_only":    true,
						"teams":       []interface{}{},
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
				Config: testAccUsersDataSourceConfig(server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_users.test", "users.#", "2"),
					resource.TestCheckResourceAttr("data.fleetdm_users.test", "users.0.name", "Alice Admin"),
					resource.TestCheckResourceAttr("data.fleetdm_users.test", "users.0.global_role", "admin"),
					resource.TestCheckResourceAttr("data.fleetdm_users.test", "users.1.name", "Bob Observer"),
					resource.TestCheckResourceAttr("data.fleetdm_users.test", "users.1.api_only", "true"),
				),
			},
		},
	})
}

func testAccUsersDataSourceConfig(serverURL string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

data "fleetdm_users" "test" {}
`
}

// TestAccUsersDataSource_live creates a user then verifies it appears in the list.
func TestAccUsersDataSource_live(t *testing.T) {
	userName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	userEmail := userName + "@example.com"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUsersDataSourceConfig_live(userName, userEmail),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.fleetdm_users.test", "users.#"),
				),
			},
		},
	})
}

func testAccUsersDataSourceConfig_live(name, email string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_user" "test" {
  name        = %[1]q
  email       = %[2]q
  password    = "FleetTest@12345!"
  global_role = "observer"
}

data "fleetdm_users" "test" {
  depends_on = [fleetdm_user.test]
}
`, name, email)
}
