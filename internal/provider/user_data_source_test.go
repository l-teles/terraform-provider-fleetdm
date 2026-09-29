package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccUserDataSource_basic(t *testing.T) {
	userName := "tf-acc-test-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	userEmail := userName + "@example.com"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserDataSourceConfig(userName, userEmail),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_user.test", "name", userName),
					resource.TestCheckResourceAttr("data.fleetdm_user.test", "email", userEmail),
					resource.TestCheckResourceAttr("data.fleetdm_user.test", "global_role", "observer"),
					resource.TestCheckResourceAttrSet("data.fleetdm_user.test", "id"),
					resource.TestCheckResourceAttr("data.fleetdm_user.test", "status", "active"),
				),
			},
		},
	})
}

func testAccUserDataSourceConfig(name, email string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_user" "test" {
  name        = %[1]q
  email       = %[2]q
  password    = "FleetTest@12345!"
  global_role = "observer"
}

data "fleetdm_user" "test" {
  id = fleetdm_user.test.id
}
`, name, email)
}

// TestAccUserDataSource_activityFields maps the Fleet 4.92 activity fields from
// a fake Fleet, both populated and null.
func TestAccUserDataSource_activityFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/fleet/users/1":
			w.Write([]byte(`{"user":{"id":1,"name":"a","email":"a@example.com","global_role":"admin",
				"last_login_at":"2026-09-01T10:00:00Z","last_activity_at":"2026-09-02T11:30:00Z","status":"active"}}`))
		case "/api/v1/fleet/users/2":
			w.Write([]byte(`{"user":{"id":2,"name":"b","email":"b@example.com","global_role":null,
				"last_login_at":null,"last_activity_at":null,"status":"no_access"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fakeFleetProviderConfig(server.URL) + `
data "fleetdm_user" "seen" {
  id = 1
}

data "fleetdm_user" "never" {
  id = 2
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.fleetdm_user.seen", "last_login_at", "2026-09-01T10:00:00Z"),
					resource.TestCheckResourceAttr("data.fleetdm_user.seen", "last_activity_at", "2026-09-02T11:30:00Z"),
					resource.TestCheckResourceAttr("data.fleetdm_user.seen", "status", "active"),
					resource.TestCheckNoResourceAttr("data.fleetdm_user.never", "last_login_at"),
					resource.TestCheckNoResourceAttr("data.fleetdm_user.never", "last_activity_at"),
					resource.TestCheckResourceAttr("data.fleetdm_user.never", "status", "no_access"),
				),
			},
		},
	})
}
