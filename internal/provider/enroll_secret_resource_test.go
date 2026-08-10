package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
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

func TestIsMaskedSecret(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{"", false},
		{"********", true},
		{"*", true},
		{"real-secret", false},
		{"**partially*masked**", false},
	}
	for _, c := range cases {
		if got := isMaskedSecret(c.value); got != c.want {
			t.Errorf("isMaskedSecret(%q) = %v, want %v", c.value, got, c.want)
		}
	}
}

func TestMatchSecretsSkipsMaskedEntries(t *testing.T) {
	r := &EnrollSecretResource{}
	planned := []EnrollSecretEntryModel{
		{Secret: types.StringValue("real-secret-1")},
	}
	api := []fleetdm.EnrollSecret{
		{Secret: "********", CreatedAt: "2026-01-01T00:00:00Z"},
		{Secret: "real-secret-1", CreatedAt: "2026-02-02T00:00:00Z"},
	}

	result := r.matchSecrets(planned, api)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Secret.ValueString() != "real-secret-1" {
		t.Errorf("expected planned secret preserved, got %q", result[0].Secret.ValueString())
	}
	if result[0].CreatedAt.ValueString() != "2026-02-02T00:00:00Z" {
		t.Errorf("expected created_at from real API entry, got %q", result[0].CreatedAt.ValueString())
	}
}

// TestReadSecretsForbiddenKeepsPlannedValues verifies that a 403 from Fleet
// (secret-read denied, possible on Fleet 4.90+) degrades to keeping the
// configured values instead of failing the operation.
func TestReadSecretsForbiddenKeepsPlannedValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "Forbidden"})
	}))
	defer server.Close()

	client, err := fleetdm.NewClient(fleetdm.ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-token",
		VerifyTLS:     false,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	r := &EnrollSecretResource{client: client}
	data := EnrollSecretResourceModel{
		Secrets: []EnrollSecretEntryModel{{Secret: types.StringValue("configured-secret")}},
	}

	var diags diag.Diagnostics
	r.readSecrets(t.Context(), &data, newEnrollDiagAdapter(&diags))
	if diags.HasError() {
		t.Fatalf("expected no errors on 403, got %v", diags.Errors())
	}
	if diags.WarningsCount() != 1 {
		t.Fatalf("expected 1 warning on 403, got %d: %v", diags.WarningsCount(), diags.Warnings())
	}
	if data.ID.ValueString() != "global" {
		t.Errorf("expected ID 'global', got %q", data.ID.ValueString())
	}
	if len(data.Secrets) != 1 || data.Secrets[0].Secret.ValueString() != "configured-secret" {
		t.Errorf("expected configured secret preserved, got %+v", data.Secrets)
	}

	// Team scope: same behavior.
	teamData := EnrollSecretResourceModel{
		TeamID:  types.Int64Value(7),
		Secrets: []EnrollSecretEntryModel{{Secret: types.StringValue("configured-team-secret")}},
	}
	var teamDiags diag.Diagnostics
	r.readSecrets(t.Context(), &teamData, newEnrollDiagAdapter(&teamDiags))
	if teamDiags.HasError() {
		t.Fatalf("expected no errors on team 403, got %v", teamDiags.Errors())
	}
	if teamDiags.WarningsCount() != 1 {
		t.Fatalf("expected 1 warning on team 403, got %d", teamDiags.WarningsCount())
	}
	if teamData.ID.ValueString() != "team-7" {
		t.Errorf("expected ID 'team-7', got %q", teamData.ID.ValueString())
	}
}

// TestReadSecretsMaskedValuesWarn verifies that masked values in a successful
// response surface a warning while keeping the configured values.
func TestReadSecretsMaskedValuesWarn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"spec": map[string]interface{}{
				"secrets": []map[string]interface{}{
					{"secret": "********", "created_at": "2026-01-01T00:00:00Z"},
				},
			},
		})
	}))
	defer server.Close()

	client, err := fleetdm.NewClient(fleetdm.ClientConfig{
		ServerAddress: server.URL,
		APIKey:        "test-token",
		VerifyTLS:     false,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	r := &EnrollSecretResource{client: client}
	data := EnrollSecretResourceModel{
		Secrets: []EnrollSecretEntryModel{{Secret: types.StringValue("configured-secret")}},
	}

	var diags diag.Diagnostics
	r.readSecrets(t.Context(), &data, newEnrollDiagAdapter(&diags))
	if diags.HasError() {
		t.Fatalf("expected no errors, got %v", diags.Errors())
	}
	if diags.WarningsCount() != 1 {
		t.Fatalf("expected 1 warning for masked values, got %d", diags.WarningsCount())
	}
	if len(data.Secrets) != 1 || data.Secrets[0].Secret.ValueString() != "configured-secret" {
		t.Errorf("expected configured secret preserved, got %+v", data.Secrets)
	}
}

// TestAccEnrollSecretResource_emptySecretRejected verifies plan-time
// validation of empty and whitespace-only secrets.
func TestAccEnrollSecretResource_emptySecretRejected(t *testing.T) {
	for name, secret := range map[string]string{"empty": "", "whitespace": "   ", "all_asterisks": "****"} {
		t.Run(name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: `
provider "fleetdm" {
  server_address = "http://localhost:1"
  api_key        = "test-token"
}

resource "fleetdm_enroll_secret" "test" {
  secrets = [{ secret = "` + secret + `" }]
}
`,
						ExpectError: regexp.MustCompile(`Invalid Enrollment Secret`),
					},
				},
			})
		})
	}
}

// TestAccEnrollSecretResource_importMaskedRejected verifies that importing
// fails cleanly when Fleet returns masked secret values (Fleet 4.90+ masks
// them for tokens without secret-read permission), instead of storing
// "********" placeholders that a later apply would push as real secrets.
func TestAccEnrollSecretResource_importMaskedRejected(t *testing.T) {
	masked := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/fleet/spec/enroll_secret" && r.Method == "GET":
			secret := "real-secret-value"
			if masked {
				secret = "********"
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"spec": map[string]interface{}{
					"secrets": []map[string]interface{}{
						{"secret": secret, "created_at": "2026-01-01T00:00:00Z"},
					},
				},
			})
		case r.URL.Path == "/api/v1/fleet/spec/enroll_secret" && r.Method == "POST":
			json.NewEncoder(w).Encode(map[string]interface{}{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	config := `
provider "fleetdm" {
  server_address = "` + server.URL + `"
  api_key        = "test-token"
}

resource "fleetdm_enroll_secret" "test" {
  secrets = [{ secret = "real-secret-value" }]
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
			},
			{
				PreConfig:     func() { masked = true },
				Config:        config,
				ResourceName:  "fleetdm_enroll_secret.test",
				ImportState:   true,
				ImportStateId: "global",
				ExpectError:   regexp.MustCompile(`Enrollment Secrets Masked`),
			},
		},
	})
}

// TestAccEnrollSecretResource_createWithSecretReadDenied reproduces the Fleet
// 4.90 scenario where the API token can write but not read secrets: apply must
// succeed (with a warning), keeping the configured values and a null
// created_at rather than failing on an unknown computed value.
func TestAccEnrollSecretResource_createWithSecretReadDenied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/fleet/spec/enroll_secret" && r.Method == "POST":
			json.NewEncoder(w).Encode(map[string]interface{}{})
		case r.URL.Path == "/api/v1/fleet/spec/enroll_secret" && r.Method == "GET":
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]interface{}{"message": "Forbidden"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccEnrollSecretResourceConfig_global(server.URL, "secret-with-read-denied"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_enroll_secret.test", "id", "global"),
					resource.TestCheckResourceAttr("fleetdm_enroll_secret.test", "secrets.0.secret", "secret-with-read-denied"),
					resource.TestCheckNoResourceAttr("fleetdm_enroll_secret.test", "secrets.0.created_at"),
				),
			},
		},
	})
}

// TestAccEnrollSecretResource_createWithMaskedReadback reproduces the Fleet
// 4.90 scenario where the read-back succeeds but returns masked values: apply
// must succeed, keeping the configured value and never storing "********".
func TestAccEnrollSecretResource_createWithMaskedReadback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/fleet/spec/enroll_secret" && r.Method == "POST":
			json.NewEncoder(w).Encode(map[string]interface{}{})
		case r.URL.Path == "/api/v1/fleet/spec/enroll_secret" && r.Method == "GET":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"spec": map[string]interface{}{
					"secrets": []map[string]interface{}{
						{"secret": "********", "created_at": "2026-01-01T00:00:00Z"},
					},
				},
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
				Config: testAccEnrollSecretResourceConfig_global(server.URL, "real-configured-secret"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_enroll_secret.test", "secrets.0.secret", "real-configured-secret"),
					resource.TestCheckNoResourceAttr("fleetdm_enroll_secret.test", "secrets.0.created_at"),
				),
			},
		},
	})
}
