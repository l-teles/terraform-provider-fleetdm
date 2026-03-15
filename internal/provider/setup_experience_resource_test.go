package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSetupExperienceResource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/fleet/setup_experience" && r.Method == "PATCH":
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/api/v1/fleet/setup_experience" && r.Method == "GET":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"enable_end_user_authentication": true,
				"enable_release_device_manually": false,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSetupExperienceResourceConfig(server.URL, 1, true, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_setup_experience.test", "team_id", "1"),
					resource.TestCheckResourceAttr("fleetdm_setup_experience.test", "enable_end_user_authentication", "true"),
					resource.TestCheckResourceAttr("fleetdm_setup_experience.test", "enable_release_device_manually", "false"),
				),
			},
		},
	})
}

func testAccSetupExperienceResourceConfig(serverURL string, teamID int, enableEndUserAuth, enableReleaseManually bool) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_setup_experience" "test" {
  team_id                        = %[2]d
  enable_end_user_authentication = %[3]t
  enable_release_device_manually = %[4]t
}
`, serverURL, teamID, enableEndUserAuth, enableReleaseManually)
}
