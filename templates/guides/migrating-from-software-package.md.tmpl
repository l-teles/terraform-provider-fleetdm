---
page_title: "Migrating from fleetdm_software_package - terraform-provider-fleetdm"
subcategory: ""
description: |-
  How to migrate from the deprecated fleetdm_software_package resource to the three type-specific resources that replace it.
---

# Migrating from `fleetdm_software_package`

`fleetdm_software_package` is deprecated. The single resource handled three
distinct Fleet API concepts (custom installers, Apple Volume Purchase Program
apps, Fleet Maintained Apps) behind a `type` discriminator. That design
caused real bugs (silently no-op'd attributes when the wrong type was set;
factorially-growing validator matrices as type-specific fields landed) and
poor UX (every attribute documented per type, half of them inapplicable for
any given block).

The replacement is three type-specific resources:

| Legacy `type`        | Replacement                                  |
| -------------------- | -------------------------------------------- |
| `"package"`          | `fleetdm_software_custom_package`            |
| `"vpp"`              | `fleetdm_software_app_store_app`             |
| `"fleet_maintained"` | `fleetdm_software_fleet_maintained_app`      |

Each new resource exposes only the attributes that actually apply to its
type. The legacy resource keeps working during the deprecation window —
you can migrate one block at a time.

## Per-resource migration recipe

The mechanical path is the same for all three types: rewrite the HCL,
detach state, re-import. This works regardless of how Terraform handles
`state mv` across schemas — see the "About `terraform state mv`" section
below.

### 1. Identify the title ID

Each existing `fleetdm_software_package` has a `title_id` you can read
from state. Make a note of it; you'll need it for the import step.

```sh
terraform state show fleetdm_software_package.example | grep title_id
# title_id = 42
```

### 2. Rewrite the HCL

#### Custom installer package — `type = "package"`

Before:

```hcl
resource "fleetdm_software_package" "example" {
  type           = "package"
  team_id        = fleetdm_team.workstations.id
  filename       = "example-app-1.0.0.pkg"
  package_path   = "${path.module}/packages/example-app-1.0.0.pkg"
  install_script = "installer -pkg /tmp/example-app-1.0.0.pkg -target /"
  self_service   = true
}
```

After:

```hcl
resource "fleetdm_software_custom_package" "example" {
  team_id        = fleetdm_team.workstations.id
  filename       = "example-app-1.0.0.pkg"
  package_path   = "${path.module}/packages/example-app-1.0.0.pkg"
  install_script = "installer -pkg /tmp/example-app-1.0.0.pkg -target /"
  self_service   = true
}
```

Drop the `type` attribute. Everything else stays the same.

#### VPP / App Store app — `type = "vpp"`

Before:

```hcl
resource "fleetdm_software_package" "xcode" {
  type         = "vpp"
  app_store_id = "497799835"
  team_id      = fleetdm_team.workstations.id
  platform     = "darwin"
  self_service = false
}
```

After:

```hcl
resource "fleetdm_software_app_store_app" "xcode" {
  app_store_id = "497799835"
  team_id      = fleetdm_team.workstations.id
  platform     = "darwin"
  self_service = false
}
```

#### Fleet Maintained App — `type = "fleet_maintained"`

Before:

```hcl
resource "fleetdm_software_package" "chrome" {
  type                    = "fleet_maintained"
  fleet_maintained_app_id = data.fleetdm_fleet_maintained_app.chrome.id
  team_id                 = fleetdm_team.workstations.id
  self_service            = true
}
```

After:

```hcl
resource "fleetdm_software_fleet_maintained_app" "chrome" {
  fleet_maintained_app_id = data.fleetdm_fleet_maintained_app.chrome.id
  team_id                 = fleetdm_team.workstations.id
  self_service            = true
}
```

### 3. Move state without destroying the Fleet resource

The legacy resource and the new resource manage the *same* Fleet title
(same `title_id`). You want to swap which Terraform resource owns it
without Fleet ever seeing a delete + recreate.

```sh
# Detach the legacy resource from state — does NOT touch Fleet.
terraform state rm fleetdm_software_package.example

# Re-import into the new resource at the same title_id.
# For a team-scoped title, append :team_id  (e.g. "42:7").
terraform import fleetdm_software_custom_package.example 42
```

### 4. Confirm

```sh
terraform plan
```

You should see no diff if the HCL matches Fleet's actual state. If the
plan wants to change `install_script`, `self_service`, or labels, that's a
hint that your HCL drifted from Fleet during the migration — apply the
diff to bring them back in sync.

## About `terraform state mv`

`terraform state mv fleetdm_software_package.example fleetdm_software_custom_package.example`
*may* work on simple cases, but the new resources have **narrower
schemas** than the legacy one — they don't accept `type`,
`app_store_id`/`fleet_maintained_app_id` (depending on direction),
`package_sha256` (for VPP/FMA), etc. Whether Terraform tolerates the
extra attributes in the moved state depends on your Terraform version
and the specific schema diff.

The `rm` + `import` recipe above is the **deterministic** path. If you
want to try `state mv` first, do it on a copy of your state and confirm
`terraform plan` shows no unexpected changes before committing.

## Notes

* The deprecated `fleetdm_software_package` continues to function during
  the deprecation window. You don't have to migrate everything in one
  go — each block can be moved independently.
* New attributes (e.g. `labels_include_all`) and bug fixes
  (e.g. `automatic_install` wiring on Fleet's package endpoints) are
  planned for follow-up PRs on the new resources. The same fixes will
  also land on the legacy resource during the deprecation window so
  you're not stranded with broken behavior while you migrate. Track
  the provider's release notes for specifics.
* The legacy resource will be removed in the next major release. Plan
  to complete migrations before that release.
