package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccBootstrapPackageResource_basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/fleet/bootstrap" && r.Method == "POST":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/api/v1/fleet/bootstrap/1/metadata" && r.Method == "GET":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"name":       "bootstrap.pkg",
				"sha256":     "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
				"token":      "test-bootstrap-token",
				"created_at": "2024-01-15T10:00:00Z",
			})
		case r.URL.Path == "/api/v1/fleet/bootstrap/1" && r.Method == "DELETE":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// A minimal valid PKG is just any binary; base64 of a small byte sequence.
	// "UEtH" is base64 for "PKG" (just used as placeholder bytes).
	const minimalPkgBase64 = "UEtH"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccBootstrapPackageResourceConfig(server.URL, 1, "bootstrap.pkg", minimalPkgBase64),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_bootstrap_package.test", "team_id", "1"),
					resource.TestCheckResourceAttr("fleetdm_bootstrap_package.test", "name", "bootstrap.pkg"),
					resource.TestCheckResourceAttr("fleetdm_bootstrap_package.test", "sha256", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"),
					resource.TestCheckResourceAttr("fleetdm_bootstrap_package.test", "token", "test-bootstrap-token"),
				),
			},
		},
	})
}

func testAccBootstrapPackageResourceConfig(serverURL string, teamID int, name, packageContent string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

resource "fleetdm_bootstrap_package" "test" {
  team_id         = ` + fmt.Sprintf("%d", teamID) + `
  name            = "` + name + `"
  package_content = "` + packageContent + `"
}
`
}
