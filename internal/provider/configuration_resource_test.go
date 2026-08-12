package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
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
