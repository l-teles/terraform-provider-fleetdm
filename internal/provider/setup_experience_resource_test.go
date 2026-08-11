package provider

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
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
