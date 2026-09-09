package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// Setup experience has no live acceptance test: Fleet gates PATCH
// /setup_experience on Apple MDM being turned on whenever the body carries
// enable_release_device_manually, manual_agent_install or
// require_all_software_macos, which this resource always does for the first of
// those. Against a Fleet started with --dev (what CI runs) every apply returns
// "MDM features aren't turned on in Fleet", so the mock tests below carry the
// coverage for the request bodies and the opt-in state handling.

func TestAccSetupExperienceResource_basic(t *testing.T) {
	mock := &setupExperienceMock{settings: map[string]any{
		"enable_end_user_authentication": true,
		"enable_release_device_manually": false,
	}}
	server := httptest.NewServer(mock.handler(t))
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
					resource.TestCheckNoResourceAttr("fleetdm_setup_experience.test", "lock_end_user_info"),
					resource.TestCheckNoResourceAttr("fleetdm_setup_experience.test", "require_all_software_macos"),
					resource.TestCheckNoResourceAttr("fleetdm_setup_experience.test", "require_all_software_windows"),
					resource.TestCheckNoResourceAttr("fleetdm_setup_experience.test", "manual_agent_install"),
				),
			},
		},
	})
}

// setupExperienceMock records every PATCH body sent to /setup_experience and
// serves the settings back the way Fleet does: on the team for a team-scoped
// resource, under mdm.macos_setup.
type setupExperienceMock struct {
	mu       sync.Mutex
	patches  []map[string]any
	settings map[string]any
}

func (m *setupExperienceMock) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/fleet/setup_experience" && r.Method == http.MethodPatch:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("failed to decode PATCH body: %v", err)
			}
			m.mu.Lock()
			m.patches = append(m.patches, body)
			m.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)

		case strings.HasPrefix(r.URL.Path, "/api/v1/fleet/fleets/") && r.Method == http.MethodGet:
			m.mu.Lock()
			settings := maps.Clone(m.settings)
			m.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"team": map[string]any{
					"mdm": map[string]any{"macos_setup": settings},
				},
			})

		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}
}

func (m *setupExperienceMock) recorded() []map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.patches
}

// setSetting changes a value out-of-band, standing in for a change made in
// Fleet's UI between two Terraform runs.
func (m *setupExperienceMock) setSetting(name string, value any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings[name] = value
}

func TestAccSetupExperienceResource_optInFields(t *testing.T) {
	mock := &setupExperienceMock{settings: map[string]any{
		"enable_end_user_authentication": true,
		"enable_release_device_manually": false,
		"lock_end_user_info":             true,
		"require_all_software_macos":     true,
		"require_all_software_windows":   false,
		"manual_agent_install":           true,
	}}
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSetupExperienceResourceConfigOptIn(server.URL, 3),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_setup_experience.test", "lock_end_user_info", "true"),
					resource.TestCheckResourceAttr("fleetdm_setup_experience.test", "require_all_software_macos", "true"),
					resource.TestCheckResourceAttr("fleetdm_setup_experience.test", "require_all_software_windows", "false"),
					resource.TestCheckResourceAttr("fleetdm_setup_experience.test", "manual_agent_install", "true"),
				),
			},
		},
	})

	patches := mock.recorded()
	if len(patches) < 2 {
		t.Fatalf("expected at least a create and a destroy PATCH, got %d", len(patches))
	}

	create := patches[0]
	wantCreate := map[string]any{
		"team_id":                        float64(3),
		"enable_end_user_authentication": true,
		"enable_release_device_manually": false,
		"lock_end_user_info":             true,
		"require_all_software_macos":     true,
		"require_all_software_windows":   false,
		"manual_agent_install":           true,
	}
	if len(create) != len(wantCreate) {
		t.Fatalf("unexpected create PATCH body: got %v, want %v", create, wantCreate)
	}
	for k, v := range wantCreate {
		if create[k] != v {
			t.Errorf("create PATCH: expected %s=%v, got %v", k, v, create[k])
		}
	}

	// Destroy resets every managed setting to false.
	destroy := patches[len(patches)-1]
	for k := range wantCreate {
		if k == "team_id" {
			continue
		}
		if destroy[k] != false {
			t.Errorf("destroy PATCH: expected %s=false, got %v", k, destroy[k])
		}
	}
}

// TestAccSetupExperienceResource_detectsDrift proves the read path observes a
// change made outside Terraform: a managed setting flipped on the mock's team
// has to show up as a non-empty plan after a refresh.
func TestAccSetupExperienceResource_detectsDrift(t *testing.T) {
	mock := &setupExperienceMock{settings: map[string]any{
		"enable_end_user_authentication": true,
		"enable_release_device_manually": false,
		"lock_end_user_info":             true,
		"require_all_software_macos":     true,
		"require_all_software_windows":   false,
		"manual_agent_install":           true,
	}}
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSetupExperienceResourceConfigOptIn(server.URL, 6),
				Check: resource.TestCheckResourceAttr(
					"fleetdm_setup_experience.test", "require_all_software_macos", "true"),
			},
			{
				PreConfig: func() {
					mock.setSetting("require_all_software_macos", false)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
				Check: resource.TestCheckResourceAttr(
					"fleetdm_setup_experience.test", "require_all_software_macos", "false"),
			},
		},
	})
}

func TestAccSetupExperienceResource_omittedOptInFieldsStayNull(t *testing.T) {
	// Fleet reports values for every opt-in setting; the ones Terraform does not
	// manage must stay out of both the request bodies and state.
	mock := &setupExperienceMock{settings: map[string]any{
		"enable_end_user_authentication": true,
		"enable_release_device_manually": false,
		"lock_end_user_info":             true,
		"require_all_software_macos":     true,
		"require_all_software_windows":   true,
		"manual_agent_install":           true,
	}}
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSetupExperienceResourceConfig(server.URL, 4, true, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("fleetdm_setup_experience.test", "lock_end_user_info"),
					resource.TestCheckNoResourceAttr("fleetdm_setup_experience.test", "require_all_software_macos"),
					resource.TestCheckNoResourceAttr("fleetdm_setup_experience.test", "require_all_software_windows"),
					resource.TestCheckNoResourceAttr("fleetdm_setup_experience.test", "manual_agent_install"),
				),
			},
		},
	})

	for i, body := range mock.recorded() {
		for _, field := range []string{
			"lock_end_user_info",
			"require_all_software_macos",
			"require_all_software_windows",
			"manual_agent_install",
		} {
			if _, ok := body[field]; ok {
				t.Errorf("PATCH %d: expected %s to be omitted, got %v", i, field, body[field])
			}
		}
	}
}

func TestAccSetupExperienceResource_lockEndUserInfoRequiresAuth(t *testing.T) {
	mock := &setupExperienceMock{settings: map[string]any{
		"enable_end_user_authentication": false,
		"enable_release_device_manually": false,
	}}
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_setup_experience" "test" {
  team_id            = 5
  lock_end_user_info = true
}
`, server.URL),
				ExpectError: regexp.MustCompile(`lock_end_user_info can only be enabled when enable_end_user_authentication`),
			},
		},
	})

	if len(mock.recorded()) != 0 {
		t.Errorf("expected no PATCH requests for an invalid configuration, got %d", len(mock.recorded()))
	}
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

func testAccSetupExperienceResourceConfigOptIn(serverURL string, teamID int) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_setup_experience" "test" {
  team_id                        = %[2]d
  enable_end_user_authentication = true
  enable_release_device_manually = false
  lock_end_user_info             = true
  require_all_software_macos     = true
  require_all_software_windows   = false
  manual_agent_install           = true
}
`, serverURL, teamID)
}

// TestAccSetupExperienceResource_managedLocalAccount covers the managed local
// admin account settings Fleet 4.91 added. They are gated on Apple MDM being
// configured — Fleet answers 422 otherwise — which the test rig now satisfies
// with self-signed APNs material, so this runs live rather than against a mock.
func TestAccSetupExperienceResource_managedLocalAccount(t *testing.T) {
	fleetName := "tf-acc-mla-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	cfg := func(enabled bool, accountType string) string {
		typeLine := ""
		if accountType != "" {
			typeLine = fmt.Sprintf("  end_user_local_account_type  = %q", accountType)
		}
		return providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name = %[1]q
}

resource "fleetdm_setup_experience" "test" {
  team_id                      = fleetdm_fleet.test.id
  enable_managed_local_account = %[2]t
%[3]s
}
`, fleetName, enabled, typeLine)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg(true, "admin"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_setup_experience.test", "enable_managed_local_account", "true"),
					resource.TestCheckResourceAttr("fleetdm_setup_experience.test", "end_user_local_account_type", "admin"),
				),
			},
			{
				Config:   cfg(true, "admin"),
				PlanOnly: true,
			},
			{
				Config: cfg(true, "standard"),
				Check: resource.TestCheckResourceAttr(
					"fleetdm_setup_experience.test", "end_user_local_account_type", "standard"),
			},
			{
				Config: cfg(true, "none"),
				Check: resource.TestCheckResourceAttr(
					"fleetdm_setup_experience.test", "end_user_local_account_type", "none"),
			},
			{
				// Turning the account off requires letting the type go back to
				// Fleet's default: it refuses "standard"/"none" while disabled.
				// Assert the type as well, or a stale "none" would pass here.
				Config: cfg(false, "admin"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_setup_experience.test", "enable_managed_local_account", "false"),
					resource.TestCheckResourceAttr("fleetdm_setup_experience.test", "end_user_local_account_type", "admin"),
				),
			},
			{
				// Import restores only id and team_id; every opt-in attribute
				// comes back null because which of them a configuration manages
				// is expressed in HCL, not on the server. Verifying them would
				// therefore fail by design, so they are ignored here and the
				// step covers the import path itself.
				ResourceName:      "fleetdm_setup_experience.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"enable_managed_local_account",
					"end_user_local_account_type",
					"enable_end_user_authentication",
					"enable_release_device_manually",
					"lock_end_user_info",
					"require_all_software_macos",
					"require_all_software_windows",
					"manual_agent_install",
				},
			},
		},
	})
}

// TestAccSetupExperienceResource_managedLocalAccountGlobal covers the managed
// local account at Fleet's global "no team" scope, which the fleet-scoped test
// cannot reach: team_id is required on this resource, and 0 is how Fleet
// addresses that scope. It mutates the global app config, so the settings are
// restored out of band afterwards rather than in a final step — a failing step
// would otherwise leave the shared instance carrying them into later tests.
func TestAccSetupExperienceResource_managedLocalAccountGlobal(t *testing.T) {
	if os.Getenv("TF_ACC") != "" {
		t.Cleanup(func() { resetGlobalManagedLocalAccountOutOfBand(t) })
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "fleetdm_setup_experience" "global" {
  team_id                      = 0
  enable_managed_local_account = true
  end_user_local_account_type  = "standard"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_setup_experience.global", "team_id", "0"),
					resource.TestCheckResourceAttr("fleetdm_setup_experience.global", "enable_managed_local_account", "true"),
					resource.TestCheckResourceAttr("fleetdm_setup_experience.global", "end_user_local_account_type", "standard"),
				),
			},
			{
				Config: providerConfig() + `
resource "fleetdm_setup_experience" "global" {
  team_id                      = 0
  enable_managed_local_account = true
  end_user_local_account_type  = "standard"
}
`,
				PlanOnly: true,
			},
		},
	})
}

// resetGlobalManagedLocalAccountOutOfBand puts the global setup experience back
// to Fleet's defaults. The account type has to be reset in the same request as
// the flag, since Fleet refuses "standard"/"none" while the account is off.
func resetGlobalManagedLocalAccountOutOfBand(t *testing.T) {
	t.Helper()
	verifyTLS := true
	if v := os.Getenv("FLEETDM_VERIFY_TLS"); v == "false" || v == "0" {
		verifyTLS = false
	}
	client, err := fleetdm.NewClient(fleetdm.ClientConfig{
		ServerAddress: os.Getenv("FLEETDM_URL"),
		APIKey:        os.Getenv("FLEETDM_API_TOKEN"),
		VerifyTLS:     verifyTLS,
	})
	if err != nil {
		t.Logf("failed to build fleet client for cleanup: %v", err)
		return
	}
	disabled, accountType := false, "admin"
	if err := client.UpdateSetupExperience(context.Background(), &fleetdm.UpdateSetupExperienceRequest{
		TeamID:                    0,
		EnableManagedLocalAccount: &disabled,
		EndUserLocalAccountType:   &accountType,
	}); err != nil {
		t.Logf("failed to reset the global managed local account: %v", err)
	}
}

// TestAccSetupExperienceResource_managedLocalAccountValidators pins the two
// plan-time rules that mirror Fleet's 422s: the account type is a fixed set,
// and "standard"/"none" require the account to be enabled.
func TestAccSetupExperienceResource_managedLocalAccountValidators(t *testing.T) {
	cfg := func(body string) string {
		return providerConfig() + fmt.Sprintf(`
resource "fleetdm_setup_experience" "test" {
  team_id = 1
%s
}
`, body)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      cfg(`  end_user_local_account_type = "root"`),
				ExpectError: regexp.MustCompile(`(?s)value\s+must\s+be\s+one\s+of`),
			},
			{
				Config: cfg(`  enable_managed_local_account = false
  end_user_local_account_type  = "standard"`),
				ExpectError: regexp.MustCompile(`(?s)enable_managed_local_account\s+must\s+be\s+true`),
			},
			{
				Config: cfg(`  enable_managed_local_account = false
  end_user_local_account_type  = "none"`),
				ExpectError: regexp.MustCompile(`(?s)enable_managed_local_account\s+must\s+be\s+true`),
			},
		},
	})
}
