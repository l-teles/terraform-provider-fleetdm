# Design note: the app-config `mdm` surface on `fleetdm_configuration`

Status: implemented. Records the shape of the global `mdm` block and the decisions behind it.

## Shape

`fleetdm_configuration` has an opt-in `mdm` block that maps the global `mdm` object of `PATCH /api/v1/fleet/config`. Its sub-blocks use Fleet's own key names, so a global setting and its per-fleet equivalent on `fleetdm_fleet` read the same:

```hcl
resource "fleetdm_configuration" "this" {
  mdm = {
    name_template = "$FLEET_VAR_HOST_HARDWARE_SERIAL"

    windows_automatic_enrollment = {
      default_fleet = fleetdm_fleet.onboarding.name
    }

    macos_updates = {
      minimum_version = "latest"
      deadline_days   = 7
    }
  }
}
```

The block follows the convention of `fleetdm_fleet`'s settings blocks: nothing is Computed, an attribute is only sent while it is configured and only refreshed while it is in state. `PATCH /config` merges the request onto the stored config, so a key left out keeps its value; this was verified live for every shared sub-block on Fleet 4.93, including moving an OS update requirement between `"latest"` and an exact version. The OS update, disk encryption and Windows update sub-blocks share their schema and validators with `fleetdm_fleet` (`internal/provider/mdm_settings_shared.go`), worded per scope.

## Scope

Fleet 4.93 has 33 keys under `mdm`. Twenty are managed by the block. The rest stay out on purpose:

- Read-only status (`enabled_and_configured`, `apple_bm_enabled_and_configured`, `apple_bm_terms_expired`, `windows_enabled_and_configured`, `android_enabled_and_configured`, `microsoft_graph_credential_invalid`) belongs on the data source.
- Settings owned by other resources or by uploaded assets: `macos_setup` (setup experience), `android_settings`, `apple_business_manager` and `volume_purchasing_program` (token assignments), and every `custom_settings` list (configuration profiles).
- Keys Fleet itself deprecates: `apple_bm_default_team`, the flat `enable_disk_encryption`, and `windows_require_bitlocker_pin`.

## Migration of the flat attributes

`host_name_template` and `windows_automatic_enrollment_default_fleet` predate the block. They stay as deprecated attributes and conflict with their in-block equivalents, so there is no precedence rule: a configuration sets each setting in one place only. Both map to the same Fleet key as the in-block attribute, so moving a value across is a no-op in Fleet. They are removed in the next major version.

## Secrets

`apple_account_provisioning.oauth_idp_client_secret` is stored by Fleet outside the app config and always read back masked. It follows the certificate authority pattern: a sensitive attribute, a write-only `_wo` sibling for Terraform 1.11 and later, and a `_wo_version` to make a rotation visible. Because the three Platform SSO settings are stored together, the secret is sent on every apply that manages the block.
