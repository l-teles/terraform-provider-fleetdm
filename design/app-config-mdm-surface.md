# Design note: the app-config `mdm` surface on `fleetdm_configuration`

Status: proposal. Records why `windows_automatic_enrollment_default_fleet` landed as a flat attribute in the Fleet 4.91 change, and what shape the rest of the surface should take.

## Where we are

`fleetdm_configuration` maps `PATCH /api/v1/fleet/config`, but only a flat subset of it. Fleet's app config has 30 keys under `mdm`; the provider exposes two, both flattened to top level:

- `host_name_template` -> `mdm.name_template`
- `windows_automatic_enrollment_default_fleet` -> `mdm.windows_automatic_enrollment.default_fleet` (added for 4.91)

Both follow the same opt-in rule: unconfigured means null, the key never goes on the wire, and a value set in the Fleet UI or by GitOps is left alone. Fleet accepts exactly one `mdm` object per request, so every managed key is merged into it at request-build time.

The flat naming was kept for the 4.91 addition deliberately. Introducing a nested `mdm` block for one new key, while `host_name_template` stayed at top level, would have given the resource two competing representations of the same API object in the same release.

## Why the flat shape does not scale

Three of the remaining keys are nested objects, not scalars (`macos_setup`, `windows_settings`, `end_user_authentication`), and three more are the per-platform OS update objects that `fleetdm_fleet` already models as `mdm.{macos,ios,ipados}_updates`. Flattening those produces names like `windows_automatic_enrollment_default_fleet` — already the longest attribute on the resource — and gives the same setting a different name depending on whether it is global or per-fleet, even though the wire shape is identical.

## Proposed shape

Add an opt-in `mdm` block whose sub-blocks mirror Fleet's own key names, so that a global setting and its per-fleet equivalent read the same:

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

This reuses the block semantics `fleetdm_fleet` already documents and tests: a block you do not declare is neither written nor read into state, and attributes inside a declared block are only sent when set. The Apple OS update sub-blocks can share `appleUpdates()` and `appleOSUpdatesConsistentValidator` with `fleetdm_fleet` rather than duplicating the `latest`/`deadline_days` rules.

## Migration

`host_name_template` and `windows_automatic_enrollment_default_fleet` become deprecated aliases of their in-block equivalents, following the `org_logo_url` / `org_logo_url_dark_mode` precedent already in this resource. Precedence, drift behaviour when both are set, and removal timing are the parts that need settling before implementation — that is the reason this is a separate change rather than part of the 4.91 work.

## Scope to confirm before implementing

Not every key belongs here. Read-only status keys (`enabled_and_configured`, `apple_bm_terms_expired`, `windows_enabled_and_configured`, `android_enabled_and_configured`) already exist on `fleetdm_configuration`'s data source and should stay computed-only. Keys backed by uploaded assets or external enrolments — `volume_purchasing_program`, `apple_business_manager`, `macos_setup`'s bootstrap package and setup-experience fields — are managed by their own resources and should not be duplicated. `macos_setup.enable_managed_local_account` is deferred separately: its wire shape was still changing in 4.91 (see the PR that added this note).
