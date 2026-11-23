package provider

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/base64"
	"encoding/binary"
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

// The mock tests below pin the request bodies and opt-in state handling; the
// live tests need the rig's self-signed Apple MDM, since Fleet gates PATCH
// /setup_experience on Apple MDM being turned on.

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
		"enable_end_user_authentication":                          true,
		"enable_release_device_manually":                          false,
		"lock_end_user_info":                                      true,
		"require_all_software_macos":                              true,
		"require_all_software_windows":                            true,
		"manual_agent_install":                                    true,
		"macos_enable_bootstrap_package_during_manual_enrollment": true,
		"macos_enable_bootstrap_package_during_migration":         true,
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
					resource.TestCheckNoResourceAttr("fleetdm_setup_experience.test", "macos_enable_bootstrap_package_during_manual_enrollment"),
					resource.TestCheckNoResourceAttr("fleetdm_setup_experience.test", "macos_enable_bootstrap_package_during_migration"),
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
			"macos_enable_bootstrap_package_during_manual_enrollment",
			"macos_enable_bootstrap_package_during_migration",
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

// TestAccSetupExperienceResource_bootstrapPackageSettings pins the Fleet 4.95
// bootstrap package settings on the wire: sent when set, read back, and reset
// to false on destroy.
func TestAccSetupExperienceResource_bootstrapPackageSettings(t *testing.T) {
	mock := &setupExperienceMock{settings: map[string]any{
		"enable_end_user_authentication":                          false,
		"enable_release_device_manually":                          false,
		"macos_enable_bootstrap_package_during_manual_enrollment": true,
		"macos_enable_bootstrap_package_during_migration":         false,
	}}
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	cfg := fmt.Sprintf(`
provider "fleetdm" {
  server_address = %q
  api_key        = "test-token"
}

resource "fleetdm_setup_experience" "test" {
  team_id = 8
  macos_enable_bootstrap_package_during_manual_enrollment = true
  macos_enable_bootstrap_package_during_migration         = false
}
`, server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_setup_experience.test", "macos_enable_bootstrap_package_during_manual_enrollment", "true"),
					resource.TestCheckResourceAttr("fleetdm_setup_experience.test", "macos_enable_bootstrap_package_during_migration", "false"),
				),
			},
			{Config: cfg, PlanOnly: true},
		},
	})

	patches := mock.recorded()
	if len(patches) < 2 {
		t.Fatalf("expected a create and a destroy PATCH, got %d", len(patches))
	}
	create, _ := json.Marshal(patches[0])
	wantCreate := `{"enable_end_user_authentication":false,"enable_release_device_manually":false,` +
		`"macos_enable_bootstrap_package_during_manual_enrollment":true,"macos_enable_bootstrap_package_during_migration":false,"team_id":8}`
	if string(create) != wantCreate {
		t.Errorf("create PATCH:\n got %s\nwant %s", create, wantCreate)
	}
	destroy := patches[len(patches)-1]
	for _, k := range []string{"macos_enable_bootstrap_package_during_manual_enrollment", "macos_enable_bootstrap_package_during_migration"} {
		if destroy[k] != false {
			t.Errorf("destroy PATCH: expected %s=false, got %v", k, destroy[k])
		}
	}
}

// TestAccSetupExperienceResource_bootstrapPackageSettingsOlderFleet covers a
// Fleet that ignores the settings and does not report them: the apply fails
// with a named error instead of recording a value Fleet never stored.
func TestAccSetupExperienceResource_bootstrapPackageSettingsOlderFleet(t *testing.T) {
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
  server_address = %q
  api_key        = "test-token"
}

resource "fleetdm_setup_experience" "test" {
  team_id = 9
  macos_enable_bootstrap_package_during_migration = true
}
`, server.URL),
				ExpectError: regexp.MustCompile(`(?s)did not report\s+macos_enable_bootstrap_package_during_migration.*Fleet\s+4\.95\.0`),
			},
		},
	})
}

// TestAccSetupExperienceResource_bootstrapPackageSettingsLive runs on a
// throwaway fleet: Fleet refuses true without a bootstrap package, and accepts
// it once one is uploaded. Fleet only checks that the package is a signed
// distribution XAR, so testBootstrapPackageBase64 is enough.
func TestAccSetupExperienceResource_bootstrapPackageSettingsLive(t *testing.T) {
	fleetName := "tf-acc-bootstrap-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	cfg := func(manual, migration bool, withPackage bool) string {
		pkg, dependsOn := "", ""
		if withPackage {
			pkg = fmt.Sprintf(`
resource "fleetdm_bootstrap_package" "test" {
  team_id         = fleetdm_fleet.test.id
  name            = "bootstrap.pkg"
  package_content = %q
}
`, testBootstrapPackageBase64(t))
			dependsOn = "depends_on = [fleetdm_bootstrap_package.test]"
		}
		return providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name = %[1]q
}
%[2]s
resource "fleetdm_setup_experience" "test" {
  team_id = fleetdm_fleet.test.id
  macos_enable_bootstrap_package_during_manual_enrollment = %[3]t
  macos_enable_bootstrap_package_during_migration         = %[4]t
  %[5]s
}
`, fleetName, pkg, manual, migration, dependsOn)
	}
	const attr = "fleetdm_setup_experience.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg(false, false, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(attr, "macos_enable_bootstrap_package_during_manual_enrollment", "false"),
					resource.TestCheckResourceAttr(attr, "macos_enable_bootstrap_package_during_migration", "false"),
				),
			},
			{Config: cfg(false, false, false), PlanOnly: true},
			{
				Config:      cfg(true, false, false),
				ExpectError: regexp.MustCompile(`(?s)first\s+specify\s+a\s+macos_bootstrap_package`),
			},
			{
				Config: cfg(true, true, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(attr, "macos_enable_bootstrap_package_during_manual_enrollment", "true"),
					resource.TestCheckResourceAttr(attr, "macos_enable_bootstrap_package_during_migration", "true"),
				),
			},
			{Config: cfg(true, true, true), PlanOnly: true},
			{
				Config: cfg(false, true, true),
				Check:  resource.TestCheckResourceAttr(attr, "macos_enable_bootstrap_package_during_manual_enrollment", "false"),
			},
		},
	})
}

// testBootstrapPackageBase64 builds a minimal XAR whose table of contents has
// a signature element and a Distribution file, which is all Fleet checks.
func testBootstrapPackageBase64(t *testing.T) string {
	t.Helper()
	toc := []byte(`<?xml version="1.0" encoding="UTF-8"?><xar><toc>` +
		`<checksum style="sha1"><offset>0</offset><size>20</size></checksum>` +
		`<signature style="RSA"><offset>20</offset><size>256</size></signature>` +
		`<file id="1"><name>Distribution</name></file></toc></xar>`)
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write(toc); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	var pkg bytes.Buffer
	header := struct {
		Magic            uint32
		HeaderSize       uint16
		Version          uint16
		CompressedSize   int64
		UncompressedSize int64
		HashType         uint32
	}{0x78617221, 28, 1, int64(compressed.Len()), int64(len(toc)), 1}
	if err := binary.Write(&pkg, binary.BigEndian, header); err != nil {
		t.Fatal(err)
	}
	pkg.Write(compressed.Bytes())
	pkg.Write(make([]byte, 276)) // heap: checksum and signature placeholders
	return base64.StdEncoding.EncodeToString(pkg.Bytes())
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
					"macos_enable_bootstrap_package_during_manual_enrollment",
					"macos_enable_bootstrap_package_during_migration",
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
