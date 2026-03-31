package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccEnrollSecretResource_global(t *testing.T) {
	// Create a mock server
	createdSecrets := []map[string]interface{}{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/api/v1/fleet/spec/enroll_secret" && r.Method == "GET":
			// Return current secrets – body: {"spec": {"secrets": [...]}}.
			json.NewEncoder(w).Encode(map[string]interface{}{
				"spec": map[string]interface{}{
					"secrets": createdSecrets,
				},
			})
			return

		case r.URL.Path == "/api/v1/fleet/spec/enroll_secret" && r.Method == "POST":
			// Apply secrets – body is {"spec": {"secrets": [...]}}.
			var body struct {
				Spec struct {
					Secrets []map[string]interface{} `json:"secrets"`
				} `json:"spec"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			createdSecrets = make([]map[string]interface{}, len(body.Spec.Secrets))
			for i, s := range body.Spec.Secrets {
				createdSecrets[i] = map[string]interface{}{
					"secret":     s["secret"],
					"created_at": "2024-01-15T10:00:00Z",
				}
			}
			json.NewEncoder(w).Encode(map[string]interface{}{})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccEnrollSecretResourceConfig_global(server.URL, "test-secret-1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_enroll_secret.test", "id", "global"),
					resource.TestCheckResourceAttr("fleetdm_enroll_secret.test", "secrets.#", "1"),
					resource.TestCheckResourceAttr("fleetdm_enroll_secret.test", "secrets.0.secret", "test-secret-1"),
					resource.TestCheckResourceAttrSet("fleetdm_enroll_secret.test", "secrets.0.created_at"),
				),
			},
			// Update testing
			{
				Config: testAccEnrollSecretResourceConfig_global_multiple(server.URL, "test-secret-1", "test-secret-2"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_enroll_secret.test", "id", "global"),
					resource.TestCheckResourceAttr("fleetdm_enroll_secret.test", "secrets.#", "2"),
					resource.TestCheckResourceAttr("fleetdm_enroll_secret.test", "secrets.0.secret", "test-secret-1"),
					resource.TestCheckResourceAttr("fleetdm_enroll_secret.test", "secrets.1.secret", "test-secret-2"),
				),
			},
			// Delete testing is automatic
		},
	})
}

func TestAccEnrollSecretResource_team(t *testing.T) {
	// Create a mock server
	teamSecrets := []map[string]interface{}{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/api/v1/fleet/fleets/1/secrets" && r.Method == "GET":
			// Return current secrets
			response := map[string]interface{}{
				"secrets": teamSecrets,
			}
			json.NewEncoder(w).Encode(response)
			return

		case r.URL.Path == "/api/v1/fleet/fleets/1/secrets" && r.Method == "PATCH":
			// Modify secrets
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			if secrets, ok := body["secrets"].([]interface{}); ok {
				teamSecrets = make([]map[string]interface{}, len(secrets))
				for i, s := range secrets {
					secretMap := s.(map[string]interface{})
					teamSecrets[i] = map[string]interface{}{
						"secret":     secretMap["secret"],
						"created_at": "2024-01-15T10:00:00Z",
						"team_id":    1,
					}
				}
			}
			response := map[string]interface{}{
				"secrets": teamSecrets,
			}
			json.NewEncoder(w).Encode(response)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccEnrollSecretResourceConfig_team(server.URL, 1, "team-secret-1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_enroll_secret.test", "id", "team-1"),
					resource.TestCheckResourceAttr("fleetdm_enroll_secret.test", "team_id", "1"),
					resource.TestCheckResourceAttr("fleetdm_enroll_secret.test", "secrets.#", "1"),
					resource.TestCheckResourceAttr("fleetdm_enroll_secret.test", "secrets.0.secret", "team-secret-1"),
				),
			},
		},
	})
}

func testAccEnrollSecretResourceConfig_global(serverURL, secret string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

resource "fleetdm_enroll_secret" "test" {
  secrets = [
    { secret = "` + secret + `" },
  ]
}
`
}

func testAccEnrollSecretResourceConfig_global_multiple(serverURL, secret1, secret2 string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

resource "fleetdm_enroll_secret" "test" {
  secrets = [
    { secret = "` + secret1 + `" },
    { secret = "` + secret2 + `" },
  ]
}
`
}

func testAccEnrollSecretResourceConfig_team(serverURL string, teamID int, secret string) string {
	return `
provider "fleetdm" {
  server_address = "` + serverURL + `"
  api_key        = "test-token"
}

resource "fleetdm_enroll_secret" "test" {
  team_id = ` + fmt.Sprintf("%d", teamID) + `
  secrets = [
    { secret = "` + secret + `" },
  ]
}
`
}
