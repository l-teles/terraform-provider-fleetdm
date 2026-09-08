package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// The tests in this file cover settings and endpoints Fleet gates on Apple MDM
// being configured. They used to be unreachable: the test rig ran with Apple
// MDM off, so every one of these answered
// 422 "... because MDM features aren't turned on in Fleet", which left them
// either mock-only or uncovered. The rig now hands Fleet self-signed APNs and
// SCEP material (see .github/fleet-test/docker-compose.yml), which is enough
// for Fleet to report Apple MDM as configured and accept these requests.
//
// What that does NOT buy: the certificates are self-signed throwaways, so no
// push notification would ever reach Apple. These exercise Fleet's API surface
// — which is what the provider talks to — not device delivery. Apple Business
// Manager and VPP remain unconfigured because they need real Apple-issued
// tokens, so ABM/DEP and App Store apps stay out of reach.

// appleLiveMobileConfig is a minimal, valid Apple configuration profile. The
// identifier is templated so parallel-safe unique profiles can be created.
func appleLiveMobileConfig(identifier, displayName string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>PayloadContent</key>
  <array/>
  <key>PayloadDisplayName</key>
  <string>%[2]s</string>
  <key>PayloadIdentifier</key>
  <string>%[1]s</string>
  <key>PayloadType</key>
  <string>Configuration</string>
  <key>PayloadUUID</key>
  <string>4f2c1b3a-5d6e-4f7a-8b9c-0d1e2f3a4b5c</string>
  <key>PayloadVersion</key>
  <integer>1</integer>
</dict>
</plist>`, identifier, displayName)
}

// TestAccConfigurationProfileResource_appleLive is the first live coverage of an
// Apple configuration profile. The existing tests for this resource are
// mock-only apart from a Windows in-place update, because uploading a
// .mobileconfig needs Apple MDM turned on.
func TestAccConfigurationProfileResource_appleLive(t *testing.T) {
	suffix := acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	fleetName := "tf-acc-apple-live-" + suffix
	identifier := "com.example.tfacc." + suffix

	cfg := func(displayName string) string {
		return providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name = %[1]q
}

resource "fleetdm_configuration_profile" "test" {
  team_id         = fleetdm_fleet.test.id
  profile_content = %[2]q
}
`, fleetName, appleLiveMobileConfig(identifier, displayName))
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg("TF Acc Apple Live"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("fleetdm_configuration_profile.test", "profile_uuid"),
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "identifier", identifier),
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "platform", "darwin"),
				),
			},
			{
				Config:   cfg("TF Acc Apple Live"),
				PlanOnly: true,
			},
			{
				// Fleet 4.90+ updates a profile in place while its identifier is
				// unchanged. Only ever verified against Windows before now.
				Config: cfg("TF Acc Apple Live Renamed"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("fleetdm_configuration_profile.test", "identifier", identifier),
					resource.TestCheckResourceAttrSet("fleetdm_configuration_profile.test", "profile_uuid"),
				),
			},
		},
	})
}

// TestAccFleetResource_recoveryLockPasswordLive is the first live coverage of
// mdm.enable_recovery_lock_password. Fleet refuses it unless Apple MDM is
// configured, so until the rig gained Apple MDM the attribute was only ever
// exercised against the mock — which cannot catch Fleet rejecting or silently
// dropping it.
func TestAccFleetResource_recoveryLockPasswordLive(t *testing.T) {
	fleetName := "tf-acc-recovery-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	cfg := func(enabled bool) string {
		return providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name = %[1]q

  mdm = {
    enable_recovery_lock_password = %[2]t
  }
}
`, fleetName, enabled)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg(true),
				Check: resource.TestCheckResourceAttr(
					"fleetdm_fleet.test", "mdm.enable_recovery_lock_password", "true"),
			},
			{
				Config:   cfg(true),
				PlanOnly: true,
			},
			{
				Config: cfg(false),
				Check: resource.TestCheckResourceAttr(
					"fleetdm_fleet.test", "mdm.enable_recovery_lock_password", "false"),
			},
		},
	})
}

// TestAccFleetResource_diskEncryptionLive covers enforcing disk encryption on a
// fleet against a live server. Fleet accepts the key without Apple MDM but only
// enforces FileVault with it configured, and the attribute had no live coverage.
func TestAccFleetResource_diskEncryptionLive(t *testing.T) {
	fleetName := "tf-acc-diskenc-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	cfg := func(enabled bool) string {
		return providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name                   = %[1]q
  enable_disk_encryption = %[2]t
}
`, fleetName, enabled)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg(true),
				Check: resource.TestCheckResourceAttr(
					"fleetdm_fleet.test", "enable_disk_encryption", "true"),
			},
			{
				Config:   cfg(true),
				PlanOnly: true,
			},
			{
				Config: cfg(false),
				Check: resource.TestCheckResourceAttr(
					"fleetdm_fleet.test", "enable_disk_encryption", "false"),
			},
		},
	})
}

// TestAccSetupExperienceResource_appleMDMRequired documents the boundary the rig
// change moved: without Apple MDM configured Fleet answers 422 on the managed
// local account, and the message names the renamed key. Asserting the message
// keeps the reason discoverable if a future rig regression turns Apple MDM back
// off — the failure then points at the rig rather than at the resource.
func TestAccSetupExperienceResource_appleMDMConfiguredInRig(t *testing.T) {
	fleetName := "tf-acc-mdmgate-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig() + fmt.Sprintf(`
resource "fleetdm_fleet" "test" {
  name = %[1]q
}

resource "fleetdm_setup_experience" "test" {
  team_id                      = fleetdm_fleet.test.id
  enable_managed_local_account = true
}
`, fleetName),
				// A rig without Apple MDM fails here with
				// "because MDM features aren't turned on in Fleet".
				Check: resource.TestCheckResourceAttr(
					"fleetdm_setup_experience.test", "enable_managed_local_account", "true"),
			},
		},
	})
}
