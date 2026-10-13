package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/l-teles/terraform-provider-fleetdm/internal/fleetdm"
)

// The global mdm object is shared by every test in the run, and several of
// these settings change how Fleet enrolls devices. Each live test therefore
// snapshots the keys it touches and restores them from a cleanup, so a failed
// step cannot leak, for example, Apple Business-only enrollment into later
// setup-experience tests.
var configurationMDMRestoredKeys = []string{
	"name_template", "windows_automatic_enrollment", "apple_server_url", "enable_recovery_lock_password",
	"macos_settings", "windows_settings", "linux_settings",
	"macos_updates", "ios_updates", "ipados_updates", "windows_updates",
	"windows_migration_enabled", "enable_turn_on_windows_mdm_manually",
	"windows_entra_tenant_ids", "windows_entra_client_ids",
	"macos_migration", "end_user_authentication",
	"apple_require_hardware_attestation", "only_allow_apple_business_enrollment",
}

// snapshotConfigurationMDM records the global mdm keys these tests change and
// registers a cleanup that writes them back.
func snapshotConfigurationMDM(t *testing.T) {
	t.Helper()
	if os.Getenv("TF_ACC") == "" || os.Getenv("FLEETDM_URL") == "" {
		return
	}
	client, err := certificateIntegrationsTestClient()
	if err != nil {
		t.Fatalf("failed to build fleet client: %v", err)
	}
	var cfg struct {
		MDM map[string]json.RawMessage `json:"mdm"`
	}
	if err := client.Get(context.Background(), "/config", nil, &cfg); err != nil {
		t.Fatalf("failed to snapshot the mdm config: %v", err)
	}
	restore := map[string]any{}
	for _, k := range configurationMDMRestoredKeys {
		v, ok := cfg.MDM[k]
		if !ok {
			continue
		}
		var decoded any
		if err := json.Unmarshal(v, &decoded); err != nil {
			t.Fatalf("failed to decode mdm.%s: %v", k, err)
		}
		restore[k] = stripReadOnlyMDMFields(k, decoded)
	}
	t.Cleanup(func() {
		// The two Apple enrollment restrictions go first and alone: they block
		// SCEP issuance for every later test, and any other key Fleet rejects
		// would fail the whole request.
		security := map[string]any{}
		for _, k := range []string{"apple_require_hardware_attestation", "only_allow_apple_business_enrollment"} {
			if v, ok := restore[k]; ok {
				security[k] = v
				delete(restore, k)
			}
		}
		if err := client.Patch(context.Background(), "/config", map[string]any{"mdm": security}, nil); err != nil {
			t.Errorf("failed to restore the Apple enrollment restrictions: %v", err)
		}
		if err := client.Patch(context.Background(), "/config", map[string]any{"mdm": restore}, nil); err != nil {
			t.Errorf("failed to restore the mdm config: %v", err)
		}
	})
}

// requireUnconfiguredAppleAccountProvisioning skips the test when Fleet
// already has Platform SSO configured, and otherwise clears it again from a
// cleanup. Fleet never returns the client secret, so a configuration that
// existed before the test could not be put back.
func requireUnconfiguredAppleAccountProvisioning(t *testing.T) {
	t.Helper()
	if os.Getenv("TF_ACC") == "" || os.Getenv("FLEETDM_URL") == "" {
		return
	}
	client, err := certificateIntegrationsTestClient()
	if err != nil {
		t.Fatalf("failed to build fleet client: %v", err)
	}
	cfg, err := client.GetAppConfig(context.Background())
	if err != nil {
		t.Fatalf("failed to read the mdm config: %v", err)
	}
	if cfg.MDM != nil && cfg.MDM.AppleAccountProvisioning != nil {
		if url := cfg.MDM.AppleAccountProvisioning.OAuthIdPTokenURL; url != nil && *url != "" {
			t.Skip("Platform SSO account provisioning is already configured on this Fleet; its secret could not be restored")
		}
	}
	t.Cleanup(func() {
		clearSecret := map[string]any{"mdm": map[string]any{"apple_account_provisioning": map[string]string{
			"oauth_idp_token_url": "", "oauth_idp_client_id": "", "oauth_idp_client_secret": "",
		}}}
		if err := client.Patch(context.Background(), "/config", clearSecret, nil); err != nil {
			t.Errorf("failed to clear apple_account_provisioning: %v", err)
		}
	})
}

// stripReadOnlyMDMFields drops the response-only parts of a snapshot that
// Fleet rejects or would misread on a PATCH.
func stripReadOnlyMDMFields(key string, v any) any {
	obj, ok := v.(map[string]any)
	if !ok {
		return v
	}
	switch key {
	case "macos_settings", "windows_settings":
		delete(obj, "custom_settings")
		delete(obj, "configuration_profiles")
	case "ios_updates", "ipados_updates":
		delete(obj, "update_new_hosts")
	}
	// An Apple update block rejects a null deadline_days next to an exact
	// version. windows_updates is sent as is: Fleet accepts null for both.
	if strings.HasSuffix(key, "os_updates") && key != "windows_updates" {
		for _, field := range []string{"deadline_days", "update_new_hosts"} {
			if obj[field] == nil {
				delete(obj, field)
			}
		}
	}
	return obj
}

func configurationMDMConfig(block string) string {
	return providerConfig() + fmt.Sprintf(`
resource "fleetdm_configuration" "test" {
  org_name = "MDM Org"
  %s
}
`, block)
}

// TestAccConfigurationResource_mdmMovesFlatAttributes moves the two deprecated
// top-level attributes into the mdm block in one apply. They map to the same
// Fleet settings, so the following plan must be empty.
func TestAccConfigurationResource_mdmMovesFlatAttributes(t *testing.T) {
	snapshotConfigurationMDM(t)
	const attr = "fleetdm_configuration.test"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: configurationMDMConfig(`host_name_template = "tf-acc-$FLEET_VAR_HOST_HARDWARE_SERIAL"
  windows_automatic_enrollment_default_fleet = ""`),
				Check: resource.TestCheckResourceAttr(attr, "host_name_template", "tf-acc-$FLEET_VAR_HOST_HARDWARE_SERIAL"),
			},
			{
				Config: configurationMDMConfig(`mdm = {
    name_template = "tf-acc-$FLEET_VAR_HOST_HARDWARE_SERIAL"
    windows_automatic_enrollment = { default_fleet = "" }
  }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(attr, "host_name_template"),
					resource.TestCheckResourceAttr(attr, "mdm.name_template", "tf-acc-$FLEET_VAR_HOST_HARDWARE_SERIAL"),
					resource.TestCheckResourceAttr(attr, "mdm.windows_automatic_enrollment.default_fleet", ""),
				),
			},
			{
				Config: configurationMDMConfig(`mdm = {
    name_template = "tf-acc-$FLEET_VAR_HOST_HARDWARE_SERIAL"
    windows_automatic_enrollment = { default_fleet = "" }
  }`),
				PlanOnly: true,
			},
			{
				Config: configurationMDMConfig(`mdm = {
    name_template = ""
  }`),
				Check: resource.TestCheckResourceAttr(attr, "mdm.name_template", ""),
			},
		},
	})
}

// TestAccConfigurationResource_mdmSharedSettings covers the settings that
// also exist per fleet: OS updates, including moving between "latest" and an
// exact version, Windows updates and per-platform disk encryption escrow.
func TestAccConfigurationResource_mdmSharedSettings(t *testing.T) {
	snapshotConfigurationMDM(t)
	const attr = "fleetdm_configuration.test"
	latest := `mdm = {
    apple_server_url = ""
    macos_updates = {
      minimum_version  = "latest"
      deadline_days    = 7
      update_new_hosts = true
    }
    ios_updates = {
      minimum_version = "latest"
      deadline_days   = 14
    }
    windows_updates = {
      deadline_days     = 7
      grace_period_days = 2
    }
    macos_settings = { enable_escrow_disk_encryption_key = true }
    linux_settings = { enable_escrow_disk_encryption_key = true }
  }`
	exact := `mdm = {
    apple_server_url = ""
    macos_updates = {
      minimum_version  = "26.6.1"
      deadline         = "2030-01-01"
      update_new_hosts = false
    }
    ios_updates = {
      minimum_version = ""
      deadline        = ""
    }
    windows_updates = {
      deadline_days     = 0
      grace_period_days = 0
    }
    macos_settings = { enable_escrow_disk_encryption_key = false }
    linux_settings = { enable_escrow_disk_encryption_key = false }
  }`
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: configurationMDMConfig(latest),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(attr, "mdm.macos_updates.minimum_version", "latest"),
					resource.TestCheckResourceAttr(attr, "mdm.macos_updates.deadline_days", "7"),
					resource.TestCheckResourceAttr(attr, "mdm.ios_updates.deadline_days", "14"),
					resource.TestCheckResourceAttr(attr, "mdm.windows_updates.grace_period_days", "2"),
					resource.TestCheckResourceAttr(attr, "mdm.macos_settings.enable_escrow_disk_encryption_key", "true"),
					resource.TestCheckResourceAttr(attr, "mdm.linux_settings.enable_escrow_disk_encryption_key", "true"),
				),
			},
			{Config: configurationMDMConfig(latest), PlanOnly: true},
			{
				Config: configurationMDMConfig(exact),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(attr, "mdm.macos_updates.minimum_version", "26.6.1"),
					resource.TestCheckNoResourceAttr(attr, "mdm.macos_updates.deadline_days"),
					resource.TestCheckResourceAttr(attr, "mdm.ios_updates.minimum_version", ""),
					resource.TestCheckResourceAttr(attr, "mdm.windows_updates.deadline_days", "0"),
				),
			},
			{Config: configurationMDMConfig(exact), PlanOnly: true},
			{
				// Dropping the block stops managing the settings without clearing them.
				Config: configurationMDMConfig(""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(attr, "mdm"),
					checkGlobalMDMInFleet("macos_updates", `"minimum_version":"26.6.1"`),
				),
			},
		},
	})
}

// TestAccConfigurationResource_mdmEnrollment covers the global-only Windows
// and macOS enrollment settings and the end-user identity provider.
func TestAccConfigurationResource_mdmEnrollment(t *testing.T) {
	snapshotConfigurationMDM(t)
	const attr = "fleetdm_configuration.test"
	const tenant = "0E1F5A2B-1234-4ABC-8DEF-0123456789AB"
	const client = "0e1f5a2b-1234-4abc-8def-0123456789ab"
	first := fmt.Sprintf(`mdm = {
    windows_migration_enabled           = true
    enable_turn_on_windows_mdm_manually = false
    windows_entra_tenant_ids            = [%q]
    windows_entra_client_ids            = [%q]
    macos_migration = {
      enable      = true
      mode        = "voluntary"
      webhook_url = "https://hook.example.test/migrate"
    }
    end_user_authentication = {
      entity_id    = "tf-acc"
      idp_name     = "TF IdP"
      metadata     = "<md:EntityDescriptor/>"
      metadata_url = ""
      issuer_uri   = ""
    }
  }`, tenant, client)
	// Swapping the two Windows toggles in one apply must work: Fleet only
	// rejects the pair when both end up true.
	second := `mdm = {
    windows_migration_enabled           = false
    enable_turn_on_windows_mdm_manually = true
    windows_entra_tenant_ids            = []
    windows_entra_client_ids            = []
    macos_migration = {
      enable      = false
      mode        = ""
      webhook_url = ""
    }
    end_user_authentication = {
      entity_id    = ""
      idp_name     = ""
      metadata     = ""
      metadata_url = ""
      issuer_uri   = ""
    }
  }`
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: configurationMDMConfig(first),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(attr, "mdm.windows_migration_enabled", "true"),
					resource.TestCheckTypeSetElemAttr(attr, "mdm.windows_entra_tenant_ids.*", tenant),
					resource.TestCheckTypeSetElemAttr(attr, "mdm.windows_entra_client_ids.*", client),
					resource.TestCheckResourceAttr(attr, "mdm.macos_migration.mode", "voluntary"),
					resource.TestCheckResourceAttr(attr, "mdm.end_user_authentication.idp_name", "TF IdP"),
				),
			},
			{Config: configurationMDMConfig(first), PlanOnly: true},
			{
				Config: configurationMDMConfig(second),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(attr, "mdm.enable_turn_on_windows_mdm_manually", "true"),
					resource.TestCheckResourceAttr(attr, "mdm.windows_entra_tenant_ids.#", "0"),
					resource.TestCheckResourceAttr(attr, "mdm.macos_migration.enable", "false"),
					resource.TestCheckResourceAttr(attr, "mdm.end_user_authentication.entity_id", ""),
				),
			},
			{Config: configurationMDMConfig(second), PlanOnly: true},
		},
	})
}

// TestAccConfigurationResource_mdmSecurityToggles turns on Apple Business-only
// enrollment and hardware attestation, which together stop Fleet issuing SCEP
// certificates, and turns them off again. The cleanup restores them even if a
// step fails. Neither needs an Apple Business token to be stored.
func TestAccConfigurationResource_mdmSecurityToggles(t *testing.T) {
	snapshotConfigurationMDM(t)
	const attr = "fleetdm_configuration.test"
	cfg := func(on bool) string {
		return configurationMDMConfig(fmt.Sprintf(`mdm = {
    apple_require_hardware_attestation   = %[1]t
    only_allow_apple_business_enrollment = %[1]t
  }`, on))
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg(true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(attr, "mdm.apple_require_hardware_attestation", "true"),
					resource.TestCheckResourceAttr(attr, "mdm.only_allow_apple_business_enrollment", "true"),
				),
			},
			{Config: cfg(true), PlanOnly: true},
			{
				Config: cfg(false),
				Check:  resource.TestCheckResourceAttr(attr, "mdm.only_allow_apple_business_enrollment", "false"),
			},
		},
	})
}

// TestAccConfigurationResource_mdmAppleAccountProvisioning covers Platform SSO
// with the client secret stored in state, which works on every Terraform
// version. Fleet reports the secret masked, so it must not show as drift.
func TestAccConfigurationResource_mdmAppleAccountProvisioning(t *testing.T) {
	requireUnconfiguredAppleAccountProvisioning(t)
	const attr = "fleetdm_configuration.test"
	cfg := func(clientID, secret string) string {
		return configurationMDMConfig(fmt.Sprintf(`mdm = {
    apple_account_provisioning = {
      oauth_idp_token_url     = "https://idp.example.test/token"
      oauth_idp_client_id     = %q
      oauth_idp_client_secret = %q
    }
  }`, clientID, secret))
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg("tf-acc-client", "tf-acc-secret-1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(attr, "mdm.apple_account_provisioning.oauth_idp_client_id", "tf-acc-client"),
					resource.TestCheckResourceAttr(attr, "mdm.apple_account_provisioning.oauth_idp_client_secret", "tf-acc-secret-1"),
				),
			},
			{Config: cfg("tf-acc-client", "tf-acc-secret-1"), PlanOnly: true},
			{
				Config: cfg("tf-acc-client-2", "tf-acc-secret-2"),
				Check:  resource.TestCheckResourceAttr(attr, "mdm.apple_account_provisioning.oauth_idp_client_id", "tf-acc-client-2"),
			},
		},
	})
}

// TestAccConfigurationResource_mdmAppleAccountProvisioningWriteOnly covers the
// write-only secret, which needs Terraform 1.11. Rotating it needs a version
// bump, because Terraform cannot see a write-only value.
func TestAccConfigurationResource_mdmAppleAccountProvisioningWriteOnly(t *testing.T) {
	requireUnconfiguredAppleAccountProvisioning(t)
	const attr = "fleetdm_configuration.test"
	cfg := func(secret string, version int) string {
		return configurationMDMConfig(fmt.Sprintf(`mdm = {
    apple_account_provisioning = {
      oauth_idp_token_url                = "https://idp.example.test/token"
      oauth_idp_client_id                = "tf-acc-client"
      oauth_idp_client_secret_wo         = %q
      oauth_idp_client_secret_wo_version = %d
    }
  }`, secret, version))
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_11_0)},
		Steps: []resource.TestStep{
			{
				Config: cfg("tf-acc-wo-1", 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(attr, "mdm.apple_account_provisioning.oauth_idp_client_secret_wo"),
					resource.TestCheckNoResourceAttr(attr, "mdm.apple_account_provisioning.oauth_idp_client_secret"),
				),
			},
			{Config: cfg("tf-acc-wo-1", 1), PlanOnly: true},
			{
				Config: cfg("tf-acc-wo-2", 2),
				Check:  resource.TestCheckResourceAttr(attr, "mdm.apple_account_provisioning.oauth_idp_client_secret_wo_version", "2"),
			},
		},
	})
}

func TestAccConfigurationResource_mdmValidators(t *testing.T) {
	cases := []struct {
		name  string
		block string
		want  string
	}{
		{"windows toggles both on", `mdm = {
    windows_migration_enabled           = true
    enable_turn_on_windows_mdm_manually = true
  }`, `cannot\s+both\s+be\s+true`},
		{"bad tenant id", `mdm = { windows_entra_tenant_ids = ["not-a-guid"] }`, `must\s+be\s+a\s+GUID`},
		{"uppercase client id", `mdm = { windows_entra_client_ids = ["0E1F5A2B-1234-4ABC-8DEF-0123456789AB"] }`, `lowercase\s+GUID`},
		{"migration without mode", `mdm = { macos_migration = {
      enable      = true
      webhook_url = "https://hook.example.test"
    } }`, `mode\s+is\s+required`},
		{"bad migration mode", `mdm = { macos_migration = { mode = "sometimes" } }`, `value\s+must\s+be\s+one\s+of`},
		{"padded idp name", `mdm = { end_user_authentication = { idp_name = " IdP " } }`, `leading\s+or\s+trailing\s+whitespace`},
		{"flat and block template", `host_name_template = "a"
  mdm = { name_template = "a" }`, `Invalid\s+Attribute\s+Combination.*cannot\s+be\s+specified`},
		{"flat and block default fleet", `windows_automatic_enrollment_default_fleet = ""
  mdm = { windows_automatic_enrollment = { default_fleet = "" } }`, `Invalid\s+Attribute\s+Combination.*cannot\s+be\s+specified`},
		{"masked secret", `mdm = { apple_account_provisioning = {
      oauth_idp_token_url     = "https://idp.example.test/token"
      oauth_idp_client_id     = "c"
      oauth_idp_client_secret = "********"
    } }`, `none\s+of`},
		{"http token url", `mdm = { apple_account_provisioning = {
      oauth_idp_token_url     = "http://idp.example.test/token"
      oauth_idp_client_id     = "c"
      oauth_idp_client_secret = "s"
    } }`, `must\s+be\s+an\s+"https://"\s+URL`},
		{"http metadata url", `mdm = { end_user_authentication = { metadata_url = "http://idp.example.test/md" } }`, `must\s+be\s+an\s+"https://"\s+URL`},
		{"http apple server url", `mdm = { apple_server_url = "http://mdm.example.test" }`, `must\s+be\s+an\s+"https://"\s+URL`},
		{"no secret", `mdm = { apple_account_provisioning = {
      oauth_idp_token_url = "https://idp.example.test/token"
      oauth_idp_client_id = "c"
    } }`, `Invalid\s+Attribute\s+Combination`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{{
					Config:      configurationMDMConfig(tc.block),
					ExpectError: regexp.MustCompile(`(?s)` + tc.want),
				}},
			})
		})
	}
}

// TestBuildConfigurationMDM pins the wire format: unmanaged keys are left out,
// an empty set goes out as [], and the secret travels with the other two
// Platform SSO settings.
func TestBuildConfigurationMDM(t *testing.T) {
	ctx := context.Background()
	m := &configurationMDMModel{
		NameTemplate:                     types.StringNull(),
		AppleServerURL:                   types.StringValue(""),
		EnableRecoveryLockPassword:       types.BoolNull(),
		WindowsMigrationEnabled:          types.BoolValue(false),
		EnableTurnOnWindowsMDMManually:   types.BoolNull(),
		WindowsEntraTenantIDs:            types.SetValueMust(types.StringType, nil),
		WindowsEntraClientIDs:            types.SetNull(types.StringType),
		AppleRequireHardwareAttestation:  types.BoolNull(),
		OnlyAllowAppleBusinessEnrollment: types.BoolValue(true),
		AppleAccountProvisioning: &configurationAppleAccountProvisioningModel{
			OAuthIdPTokenURL: types.StringValue("https://idp.example.test/token"),
			OAuthIdPClientID: types.StringValue("c"),
		},
	}
	req := &fleetdm.UpdateAppConfigRequest{}
	var diags diag.Diagnostics
	buildConfigurationMDM(ctx, m, new("s3cret"), req, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	raw, err := json.Marshal(req.MDM)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	want := `{"apple_server_url":"","windows_migration_enabled":false,"windows_entra_tenant_ids":[],` +
		`"apple_account_provisioning":{"oauth_idp_token_url":"https://idp.example.test/token","oauth_idp_client_id":"c","oauth_idp_client_secret":"s3cret"},` +
		`"only_allow_apple_business_enrollment":true}`
	if got != want {
		t.Errorf("wire format:\n got %s\nwant %s", got, want)
	}

	var none fleetdm.UpdateAppConfigRequest
	buildConfigurationMDM(ctx, nil, nil, &none, &diags)
	if none.MDM != nil {
		t.Errorf("an undeclared block must not add an mdm object, got %+v", none.MDM)
	}
}

// TestConfigurationMDMOnOlderFleet covers a server that drops keys it does
// not know: refresh keeps state, and apply names the missing settings.
func TestConfigurationMDMOnOlderFleet(t *testing.T) {
	m := &configurationMDMModel{
		AppleServerURL:                   types.StringNull(),
		WindowsMigrationEnabled:          types.BoolNull(),
		EnableTurnOnWindowsMDMManually:   types.BoolNull(),
		AppleRequireHardwareAttestation:  types.BoolValue(true),
		OnlyAllowAppleBusinessEnrollment: types.BoolValue(true),
	}
	var api fleetdm.MDMConfig
	if err := json.Unmarshal([]byte(`{"enabled_and_configured":true,"apple_require_hardware_attestation":true}`), &api); err != nil {
		t.Fatal(err)
	}
	var diags diag.Diagnostics
	refreshConfigurationMDM(m, &api, &diags)
	if !m.OnlyAllowAppleBusinessEnrollment.ValueBool() {
		t.Error("refresh must keep a setting the server did not report")
	}
	checkConfigurationMDMApplied(m, &api, &diags)
	if !diags.HasError() {
		t.Fatal("expected an error naming the unsupported setting")
	}
	msg := diags.Errors()[0].Detail()
	if !strings.Contains(msg, "only_allow_apple_business_enrollment") || strings.Contains(msg, "apple_require_hardware_attestation") {
		t.Errorf("expected only the missing setting to be named, got %q", msg)
	}
}

// checkGlobalMDMInFleet asserts a fragment of a global mdm key directly, for
// steps where Terraform no longer manages it.
func checkGlobalMDMInFleet(key, fragment string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		client, err := certificateIntegrationsTestClient()
		if err != nil {
			return err
		}
		var cfg struct {
			MDM map[string]json.RawMessage `json:"mdm"`
		}
		if err := client.Get(context.Background(), "/config", nil, &cfg); err != nil {
			return err
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, cfg.MDM[key]); err != nil {
			return err
		}
		if !strings.Contains(compact.String(), fragment) {
			return fmt.Errorf("mdm.%s is %s, want it to contain %s", key, cfg.MDM[key], fragment)
		}
		return nil
	}
}

// TestMacOSOnlyUpdateNewHostsValidator checks that update_new_hosts on iOS and
// iPadOS, which Fleet discards on both endpoints, warns instead of passing
// silently, and that leaving it out stays quiet.
func TestMacOSOnlyUpdateNewHostsValidator(t *testing.T) {
	for _, tc := range []struct {
		value    types.Bool
		warnings int
	}{
		{types.BoolValue(true), 1},
		{types.BoolValue(false), 1},
		{types.BoolNull(), 0},
	} {
		var resp validator.BoolResponse
		macOSOnlyUpdateNewHostsValidator{}.ValidateBool(context.Background(), validator.BoolRequest{ConfigValue: tc.value}, &resp)
		if got := resp.Diagnostics.WarningsCount(); got != tc.warnings || resp.Diagnostics.HasError() {
			t.Errorf("%v: got %d warnings (errors: %v), want %d", tc.value, got, resp.Diagnostics.HasError(), tc.warnings)
		}
	}
}

// TestCheckConfigurationMDMAppliedLists covers the list and update keys: a
// list Fleet did not report at all is named, while an empty one is not.
func TestCheckConfigurationMDMAppliedLists(t *testing.T) {
	m := &configurationMDMModel{
		AppleServerURL:                   types.StringNull(),
		EnableRecoveryLockPassword:       types.BoolNull(),
		WindowsMigrationEnabled:          types.BoolNull(),
		EnableTurnOnWindowsMDMManually:   types.BoolNull(),
		AppleRequireHardwareAttestation:  types.BoolNull(),
		OnlyAllowAppleBusinessEnrollment: types.BoolNull(),
		WindowsEntraTenantIDs:            types.SetValueMust(types.StringType, nil),
		WindowsEntraClientIDs:            types.SetValueMust(types.StringType, nil),
	}
	var present fleetdm.MDMConfig
	if err := json.Unmarshal([]byte(`{"windows_entra_tenant_ids":[],"windows_entra_client_ids":[]}`), &present); err != nil {
		t.Fatal(err)
	}
	// Refresh keeps a list Fleet did not report and clears one it reported as [].
	m.WindowsEntraTenantIDs = types.SetValueMust(types.StringType, []attr.Value{types.StringValue("x")})
	var rd diag.Diagnostics
	refreshConfigurationMDM(m, &fleetdm.MDMConfig{}, &rd)
	if len(m.WindowsEntraTenantIDs.Elements()) != 1 {
		t.Errorf("an unreported list must keep state, got %v", m.WindowsEntraTenantIDs)
	}
	refreshConfigurationMDM(m, &present, &rd)
	if len(m.WindowsEntraTenantIDs.Elements()) != 0 {
		t.Errorf("a list reported as [] must clear state, got %v", m.WindowsEntraTenantIDs)
	}
	var ok diag.Diagnostics
	checkConfigurationMDMApplied(m, &present, &ok)
	if ok.HasError() {
		t.Errorf("empty lists Fleet reported must not be flagged: %v", ok)
	}
	var missing diag.Diagnostics
	checkConfigurationMDMApplied(m, &fleetdm.MDMConfig{}, &missing)
	if !missing.HasError() || !strings.Contains(missing.Errors()[0].Detail(), "windows_entra_client_ids") {
		t.Errorf("expected the unreported lists to be named, got %v", missing)
	}
}
