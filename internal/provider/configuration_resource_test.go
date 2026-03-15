package provider

import (
	"fmt"
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
			// Set new fields explicitly.
			// Note: enable_analytics must stay true because Fleet's --dev mode
			// forces it on and ignores attempts to disable it.
			{
				Config: testAccConfigurationResourceConfigNewFields(
					"New Fields Test Org",
					true,                                 // enable_analytics (forced true in dev mode)
					true,                                 // ai_features_disabled
					"https://example.com/light-logo.png", // org_logo_url_light_background
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "org_name", "New Fields Test Org"),
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "enable_analytics", "true"),
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "ai_features_disabled", "true"),
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "org_logo_url_light_background", "https://example.com/light-logo.png"),
				),
			},
			// Update – toggle ai_features and clear the logo.
			{
				Config: testAccConfigurationResourceConfigNewFields(
					"New Fields Test Org",
					true,  // enable_analytics
					false, // ai_features_disabled
					"",    // org_logo_url_light_background
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "enable_analytics", "true"),
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "ai_features_disabled", "false"),
					resource.TestCheckResourceAttr("fleetdm_configuration.test", "org_logo_url_light_background", ""),
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
