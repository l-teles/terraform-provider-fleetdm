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
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

func TestAccConfigurationResource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read – set explicit values so we can assert on them.
			{
				Config: testAccConfigurationResourceConfig("Terraform Acc Test Org", false, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "id", "configuration"),
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "org_name", "Terraform Acc Test Org"),
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "live_query_disabled", "false"),
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "scripts_disabled", "false"),
					resource.TestCheckResourceAttrSet("fleetdm_configuration.test", "server_url"),
				),
			},
			// ImportState
			{
				ResourceName:      "fleetdm_configuration.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update – change org name and toggle a flag.
			{
				Config: testAccConfigurationResourceConfig("Terraform Acc Test Org Updated", true, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "org_name", "Terraform Acc Test Org Updated"),
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "live_query_disabled", "true"),
				),
			},
		},
	})
}

func TestAccConfigurationResource_hostExpiry(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccConfigurationResourceConfigHostExpiry("Expiry Test Org", true, 45),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "org_name", "Expiry Test Org"),
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "host_expiry_enabled", "true"),
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "host_expiry_window", "45"),
				),
			},
			// Disable host expiry – Fleet preserves the window value even when disabled.
			{
				Config: testAccConfigurationResourceConfigHostExpiry("Expiry Test Org", false, 45),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "host_expiry_enabled", "false"),
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "host_expiry_window", "45"),
				),
			},
		},
	})
}

func testAccConfigurationResourceConfig(orgName string, liveQueryDisabled, scriptsDisabled bool) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_configuration" "test" {
  org_name            = %[1]q
  live_query_disabled = %[2]t
  scripts_disabled    = %[3]t
}
`, orgName, liveQueryDisabled, scriptsDisabled)
}

func TestAccConfigurationResource_newFields(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Set new fields via the deprecated org_logo_url_light_background alias.
			// Note: enable_analytics must stay true because Fleet's --dev mode
			// forces it on and ignores attempts to disable it.
			{
				Config: testAccConfigurationResourceConfigNewFields(
					"New Fields Test Org",
					true,                                 // enable_analytics (forced true in dev mode)
					true,                                 // ai_features_disabled
					"https://example.com/light-logo.png", // org_logo_url_light_background (deprecated alias)
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "org_name", "New Fields Test Org"),
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "enable_analytics", "true"),
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "ai_features_disabled", "true"),
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "org_logo_url_light_background", "https://example.com/light-logo.png"),
					// The deprecated alias is mirrored to the canonical *_mode field.
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "org_logo_url_light_mode", "https://example.com/light-logo.png"),
				),
			},
			// Update – toggle ai_features and change the logo to a different URL via
			// the deprecated alias. The provider translates the alias to the canonical
			// org_logo_url_light_mode key, so changing it works on Fleet >= 4.86.
			{
				Config: testAccConfigurationResourceConfigNewFields(
					"New Fields Test Org",
					true,                                   // enable_analytics
					false,                                  // ai_features_disabled
					"https://example.com/light-logo-2.png", // org_logo_url_light_background (changed)
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "enable_analytics", "true"),
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "ai_features_disabled", "false"),
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "org_logo_url_light_background", "https://example.com/light-logo-2.png"),
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "org_logo_url_light_mode", "https://example.com/light-logo-2.png"),
				),
			},
		},
	})
}

func testAccConfigurationResourceConfigHostExpiry(orgName string, enabled bool, window int) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_configuration" "test" {
  org_name            = %[1]q
  host_expiry_enabled = %[2]t
  host_expiry_window  = %[3]d
}
`, orgName, enabled, window)
}

func testAccConfigurationResourceConfigNewFields(orgName string, enableAnalytics, aiFeaturesDisabled bool, orgLogoURLLightBg string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_configuration" "test" {
  org_name                       = %[1]q
  enable_analytics               = %[2]t
  ai_features_disabled           = %[3]t
  org_logo_url_light_background  = %[4]q
}
`, orgName, enableAnalytics, aiFeaturesDisabled, orgLogoURLLightBg)
}

// TestAccConfigurationResource_logoModeFields exercises the canonical
// org_logo_url_dark_mode / org_logo_url_light_mode fields — including changing
// them — and verifies that the deprecated aliases mirror their values.
func TestAccConfigurationResource_logoModeFields(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccConfigurationResourceConfigLogoModes(
					"Logo Modes Org",
					"https://example.com/dark-1.png",
					"https://example.com/light-1.png",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "org_logo_url_dark_mode", "https://example.com/dark-1.png"),
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "org_logo_url_light_mode", "https://example.com/light-1.png"),
					// Deprecated aliases mirror the canonical fields.
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "org_logo_url", "https://example.com/dark-1.png"),
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "org_logo_url_light_background", "https://example.com/light-1.png"),
				),
			},
			{
				Config: testAccConfigurationResourceConfigLogoModes(
					"Logo Modes Org",
					"https://example.com/dark-2.png",
					"https://example.com/light-2.png",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "org_logo_url_dark_mode", "https://example.com/dark-2.png"),
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "org_logo_url_light_mode", "https://example.com/light-2.png"),
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "org_logo_url", "https://example.com/dark-2.png"),
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "org_logo_url_light_background", "https://example.com/light-2.png"),
				),
			},
		},
	})
}

func testAccConfigurationResourceConfigLogoModes(orgName, dark, light string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_configuration" "test" {
  org_name                = %[1]q
  org_logo_url_dark_mode  = %[2]q
  org_logo_url_light_mode = %[3]q
}
`, orgName, dark, light)
}

// TestAccConfigurationResource_logoConflict verifies that setting a canonical
// *_mode field and its deprecated alias to different values is rejected before
// any request is sent to Fleet.
func TestAccConfigurationResource_logoConflict(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "fleetdm_configuration" "test" {
  org_name               = "Logo Conflict Org"
  org_logo_url_dark_mode = "https://example.com/dark.png"
  org_logo_url           = "https://example.com/different.png"
}
`,
				ExpectError: regexp.MustCompile(`Conflicting organization logo configuration`),
			},
		},
	})
}

// TestAccConfigurationResource_hostNameTemplate exercises the global ("No team")
// host name template: setting it, changing it, and clearing it with "".
//
// The final step must leave the template cleared. This is a singleton resource
// on a shared Fleet instance and Delete deliberately does not revert anything,
// so a template left behind would apply to every "No team" host for the rest of
// the run.
func TestAccConfigurationResource_hostNameTemplate(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccConfigurationResourceConfigHostNameTemplate(
					"Host Name Template Org", `tf-acc-$FLEET_VAR_HOST_HARDWARE_SERIAL`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "host_name_template",
						"tf-acc-$FLEET_VAR_HOST_HARDWARE_SERIAL"),
				),
			},
			// Change the template to a different supported variable.
			{
				Config: testAccConfigurationResourceConfigHostNameTemplate(
					"Host Name Template Org", `tf-acc-$FLEET_VAR_HOST_UUID`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "host_name_template",
						"tf-acc-$FLEET_VAR_HOST_UUID"),
				),
			},
			// Clear it, which also restores the shared instance for other tests.
			{
				Config: testAccConfigurationResourceConfigHostNameTemplate("Host Name Template Org", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "host_name_template", ""),
				),
			},
		},
	})
}

// TestAccConfigurationResource_hostNameTemplateUnmanaged verifies the opt-in
// convention: a configuration that never mentions host_name_template leaves the
// attribute null rather than absorbing Fleet's value into state.
func TestAccConfigurationResource_hostNameTemplateUnmanaged(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "fleetdm_configuration" "test" {
  org_name = "Unmanaged Template Org"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("fleetdm_configuration.test", "host_name_template"),
				),
			},
		},
	})
}

// TestAccConfigurationResource_hostNameTemplatePadded checks that a template with
// surrounding whitespace is rejected at plan time. Fleet would silently store the
// trimmed form, which Terraform reports as an inconsistent result after apply.
func TestAccConfigurationResource_hostNameTemplatePadded(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccConfigurationResourceConfigHostNameTemplate(
					"Padded Template Org", `  tf-acc-padded  `),
				// Diagnostics are line-wrapped, so match across arbitrary whitespace.
				ExpectError: regexp.MustCompile(`Padded\s+Host\s+Name\s+Template`),
			},
		},
	})
}

func testAccConfigurationResourceConfigHostNameTemplate(orgName, template string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_configuration" "test" {
  org_name           = %[1]q
  host_name_template = %[2]q
}
`, orgName, template)
}

// TestAccConfigurationResource_windowsAutomaticEnrollmentDefaultFleet covers the
// Fleet 4.91 default fleet for user-driven Windows MDM enrollment. Fleet takes a
// fleet name here, resolves it on write, and rejects an unknown one.
func TestAccConfigurationResource_windowsAutomaticEnrollmentDefaultFleet(t *testing.T) {
	fleetName := "tf-acc-wae-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	cfg := func(defaultFleet string) string {
		return providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "target" {
  name = %[1]q
}

resource "fleetdm_configuration" "test" {
  org_name                                   = "Windows Autopilot Org"
  windows_automatic_enrollment_default_fleet = %[2]s
}
`, fleetName, defaultFleet)
	}

	// app config is a global singleton shared by the whole serial suite, and
	// the fleet this points at is destroyed at teardown. Clear it from a
	// cleanup rather than only in a final step: if an earlier step fails, that
	// step never runs, and Fleet then rejects every later app-config write that
	// re-sends a name whose fleet no longer exists.
	if os.Getenv("TF_ACC") != "" {
		t.Cleanup(func() { clearWindowsAutomaticEnrollmentOutOfBand(t) })
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg("fleetdm_fleet.target.name"),
				Check: resource.TestCheckResourceAttr("fleetdm_configuration.test",
					"windows_automatic_enrollment_default_fleet", fleetName),
			},
			{
				Config:   cfg("fleetdm_fleet.target.name"),
				PlanOnly: true,
			},
			{
				// "" is how Fleet expresses "no default": new hosts stay unassigned.
				// Also restores the shared instance for the other tests.
				Config: cfg(`""`),
				Check: resource.TestCheckResourceAttr("fleetdm_configuration.test",
					"windows_automatic_enrollment_default_fleet", ""),
			},
		},
	})
}

// TestAccConfigurationResource_windowsAutomaticEnrollmentUnknownFleet pins
// Fleet's rejection of a name that does not resolve to a fleet.
func TestAccConfigurationResource_windowsAutomaticEnrollmentUnknownFleet(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "fleetdm_configuration" "test" {
  org_name                                   = "Windows Autopilot Org"
  windows_automatic_enrollment_default_fleet = "tf-acc-no-such-fleet-xyz"
}
`,
				ExpectError: regexp.MustCompile(`(?s)doesn't\s+exist`),
			},
		},
	})
}

// TestAccConfigurationResource_windowsAutomaticEnrollmentUnmanaged verifies the
// opt-in convention holds for the new attribute too: a configuration that never
// mentions it leaves it null instead of absorbing Fleet's value.
func TestAccConfigurationResource_windowsAutomaticEnrollmentUnmanaged(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + `
resource "fleetdm_configuration" "test" {
  org_name = "Unmanaged Autopilot Org"
}
`,
				Check: resource.TestCheckNoResourceAttr("fleetdm_configuration.test",
					"windows_automatic_enrollment_default_fleet"),
			},
		},
	})
}

// clearWindowsAutomaticEnrollmentOutOfBand resets the global default fleet for
// Windows MDM enrollment. Fleet rejects a default_fleet naming a fleet that no
// longer exists, so leaving one behind would break later app-config writes in
// the same run.
func clearWindowsAutomaticEnrollmentOutOfBand(t *testing.T) {
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
	_, err = client.UpdateAppConfig(context.Background(), &fleetdm.UpdateAppConfigRequest{
		MDM: &fleetdm.MDMSettingsUpdate{
			WindowsAutomaticEnrollment: &fleetdm.WindowsAutomaticEnrollment{DefaultFleet: ""},
		},
	})
	if err != nil {
		t.Logf("failed to clear windows_automatic_enrollment.default_fleet: %v", err)
	}
}

// TestAccConfigurationResource_agentOptions sets the global agent options,
// including a top-level key, and checks that Fleet's copy causes no drift.
func TestAccConfigurationResource_agentOptions(t *testing.T) {
	config := providerConfig() + `
resource "fleetdm_configuration" "test" {
  org_name = "Terraform Acc Test Org"
  agent_options = jsonencode({
    script_execution_timeout = 600
    config = {
      options = {
        pack_delimiter               = "/"
        logger_tls_period            = 10
        distributed_plugin           = "tls"
        disable_distributed          = false
        logger_tls_endpoint          = "/api/osquery/log"
        distributed_interval         = 10
        distributed_tls_max_attempts = 3
      }
      decorators = {
        load = [
          "SELECT uuid AS host_uuid FROM system_info;",
          "SELECT hostname AS hostname FROM system_info;",
        ]
      }
    }
    overrides = {}
  })
}
`

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  checkAgentOptionsTimeout("fleetdm_configuration.test", 600),
			},
			{
				Config:   config,
				PlanOnly: true,
			},
		},
	})
}

// appConfigMock serves /config from a map that each PATCH merges into, one
// section deep, and records every server_settings object sent. dropMCP stands
// in for a Fleet that predates mcp_disabled.
type appConfigMock struct {
	mu             sync.Mutex
	dropMCP        bool
	config         map[string]map[string]any
	serverSettings []string
}

func newAppConfigMock(dropMCP bool) *appConfigMock {
	return &appConfigMock{dropMCP: dropMCP, config: map[string]map[string]any{
		"org_info":                 {"org_name": "", "contact_url": "https://fleetdm.com/company/contact"},
		"server_settings":          {"server_url": "https://fleet.example.test"},
		"host_expiry_settings":     {},
		"activity_expiry_settings": {},
		"features":                 {"enable_host_users": true, "enable_software_inventory": true},
		"fleet_desktop":            {},
	}}
}

func (m *appConfigMock) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fleet/config" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if r.Method == http.MethodPatch {
			var body map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("failed to decode PATCH body: %v", err)
			}
			m.serverSettings = append(m.serverSettings, string(body["server_settings"]))
			for section, raw := range body {
				var values map[string]any
				if err := json.Unmarshal(raw, &values); err != nil {
					continue
				}
				if m.config[section] == nil {
					m.config[section] = map[string]any{}
				}
				maps.Copy(m.config[section], values)
			}
			if m.dropMCP {
				delete(m.config["server_settings"], "mcp_disabled")
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(m.config)
	}
}

func mcpDisabledMockConfig(serverURL, line string) string {
	return fmt.Sprintf(`
provider "fleetdm" {
  server_address = %[1]q
  api_key        = "test-token"
}

resource "fleetdm_configuration" "test" {
  org_name = "Mock Org"
  %[2]s
}
`, serverURL, line)
}

// TestAccConfigurationResource_mcpDisabledWire pins the wire format: the key is
// sent only while managed, and dropping it from HCL leaves Fleet's value alone
// without drift.
func TestAccConfigurationResource_mcpDisabledWire(t *testing.T) {
	mock := newAppConfigMock(false)
	server := httptest.NewServer(mock.handler(t))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: mcpDisabledMockConfig(server.URL, "mcp_disabled = true"),
				Check:  resource.TestCheckResourceAttr("fleetdm_configuration.test", "mcp_disabled", "true"),
			},
			{
				Config: mcpDisabledMockConfig(server.URL, ""),
				Check:  resource.TestCheckNoResourceAttr("fleetdm_configuration.test", "mcp_disabled"),
			},
			{Config: mcpDisabledMockConfig(server.URL, ""), PlanOnly: true},
		},
	})

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.serverSettings) != 2 {
		t.Fatalf("expected two PATCH requests, got %d: %v", len(mock.serverSettings), mock.serverSettings)
	}
	const base = `{"live_query_disabled":false,"enable_analytics":true,"query_reports_disabled":false,"scripts_disabled":false,"ai_features_disabled":false`
	if want := base + `,"mcp_disabled":true}`; mock.serverSettings[0] != want {
		t.Errorf("managed:\n got %s\nwant %s", mock.serverSettings[0], want)
	}
	if want := base + `}`; mock.serverSettings[1] != want {
		t.Errorf("unmanaged:\n got %s\nwant %s", mock.serverSettings[1], want)
	}
	if mock.config["server_settings"]["mcp_disabled"] != true {
		t.Errorf("removing the attribute must leave Fleet's value alone, got %v", mock.config["server_settings"]["mcp_disabled"])
	}
}

// TestAccConfigurationResource_mcpDisabledOlderFleet covers a Fleet that does
// not report mcp_disabled: the apply fails with a named error.
func TestAccConfigurationResource_mcpDisabledOlderFleet(t *testing.T) {
	server := httptest.NewServer(newAppConfigMock(true).handler(t))
	defer server.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      mcpDisabledMockConfig(server.URL, "mcp_disabled = false"),
				ExpectError: regexp.MustCompile(`(?s)did not report mcp_disabled.*Fleet 4\.95\.0`),
			},
		},
	})
}

// TestAccConfigurationResource_mcpDisabled toggles the MCP server on a live
// Fleet and restores the original value out of band.
func TestAccConfigurationResource_mcpDisabled(t *testing.T) {
	restoreServerSettingOutOfBand(t, "mcp_disabled")
	const attr = "fleetdm_configuration.test"
	cfg := func(line string) string {
		return providerConfig() + fmt.Sprintf(`
resource "fleetdm_configuration" "test" {
  org_name = "MCP Org"
  %s
}
`, line)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg("mcp_disabled = true"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(attr, "mcp_disabled", "true"),
					checkServerSettingInFleet("mcp_disabled", true),
				),
			},
			{Config: cfg("mcp_disabled = true"), PlanOnly: true},
			{
				Config: cfg("mcp_disabled = false"),
				Check:  checkServerSettingInFleet("mcp_disabled", false),
			},
			{
				// Unmanaged again: Fleet keeps false and the plan stays empty.
				Config: cfg(""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(attr, "mcp_disabled"),
					checkServerSettingInFleet("mcp_disabled", false),
				),
			},
			{Config: cfg(""), PlanOnly: true},
		},
	})
}

// restoreServerSettingOutOfBand snapshots one global server setting and
// writes it back from a cleanup, since other tests share the instance.
func restoreServerSettingOutOfBand(t *testing.T, key string) {
	t.Helper()
	if os.Getenv("TF_ACC") == "" || os.Getenv("FLEETDM_URL") == "" {
		return
	}
	client, err := certificateIntegrationsTestClient()
	if err != nil {
		t.Fatalf("failed to build fleet client: %v", err)
	}
	var cfg struct {
		ServerSettings map[string]json.RawMessage `json:"server_settings"`
	}
	if err := client.Get(context.Background(), "/config", nil, &cfg); err != nil {
		t.Fatalf("failed to snapshot server_settings: %v", err)
	}
	original, ok := cfg.ServerSettings[key]
	if !ok {
		return
	}
	t.Cleanup(func() {
		body := map[string]any{"server_settings": map[string]json.RawMessage{key: original}}
		if err := client.Patch(context.Background(), "/config", body, nil); err != nil {
			t.Errorf("failed to restore server_settings.%s: %v", key, err)
		}
	})
}

// checkServerSettingInFleet asserts a global server setting directly.
func checkServerSettingInFleet(key string, want bool) resource.TestCheckFunc {
	return func(*terraform.State) error {
		client, err := certificateIntegrationsTestClient()
		if err != nil {
			return err
		}
		var cfg struct {
			ServerSettings map[string]any `json:"server_settings"`
		}
		if err := client.Get(context.Background(), "/config", nil, &cfg); err != nil {
			return err
		}
		if got := cfg.ServerSettings[key]; got != want {
			return fmt.Errorf("server_settings.%s is %v, want %v", key, got, want)
		}
		return nil
	}
}
