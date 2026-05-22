---
page_title: "Resource fleetdm_software_package - fleetdm"
subcategory: ""
description: |-
    Manages a FleetDM software package, VPP (App Store) app, or Fleet Maintained App. This is a Premium feature.
---

# fleetdm_software_package (Resource)

Manages a FleetDM software package, VPP (App Store) app, or Fleet Maintained App. This is a Premium feature.

## Example Usage

```terraform
# --- type = "package" (default) ---
# Upload a software installer from a local file.
# The package is re-uploaded only when the file content (SHA256) changes.

resource "fleetdm_software_package" "example_app" {
  team_id = fleetdm_team.workstations.id

  filename     = "example-app-1.0.0.pkg"
  package_path = "${path.module}/packages/example-app-1.0.0.pkg"

  install_script      = "installer -pkg /tmp/example-app-1.0.0.pkg -target /"
  uninstall_script    = "rm -rf /Applications/ExampleApp.app"
  post_install_script = "open /Applications/ExampleApp.app"

  self_service      = true
  automatic_install = false
}

# Package with a pre-install query and label targeting
resource "fleetdm_software_package" "developer_tools" {
  team_id = fleetdm_team.engineering.id

  filename     = "dev-tools.pkg"
  package_path = "${path.module}/packages/dev-tools.pkg"

  # Only install if the tool is not already present
  pre_install_query = "SELECT 1 FROM apps WHERE name != 'DevTools';"

  install_script = "installer -pkg /tmp/dev-tools.pkg -target /"

  labels_include_any = ["Developers", "Engineers"]
  labels_exclude_any = ["Contractors"]

  self_service = true
}

# --- type = "package" with S3 source ---
# Download the installer from S3 instead of a local file.
# Useful for CI/CD pipelines on ephemeral runners.
#
# Fast path: when the S3 object has a SHA256 checksum the provider can read via
# HeadObject, terraform apply on an unchanged installer is a no-op (no body
# download, no re-upload). S3 does NOT compute SHA256 by default — to enable
# the fast path, either:
#
#   (a) Upload with `--checksum-algorithm SHA256` in a single part:
#         aws s3api put-object \
#           --bucket my-software-bucket \
#           --key installers/example-app-1.0.0.pkg \
#           --body example-app-1.0.0.pkg \
#           --checksum-algorithm SHA256
#
#   (b) Set the SHA in object metadata:
#         aws s3 cp example-app-1.0.0.pkg s3://my-software-bucket/... \
#           --metadata sha256=$(sha256sum example-app-1.0.0.pkg | cut -d' ' -f1)
#
#   (c) Set package_s3.expected_sha256 in this Terraform config (see below).
#
# If none of those is set, the provider falls back to downloading the body on
# every apply (a warning is emitted explaining how to opt-in).

resource "aws_s3_object" "example_app" {
  bucket             = "my-software-bucket"
  key                = "installers/example-app-1.0.0.pkg"
  source             = "${path.module}/packages/example-app-1.0.0.pkg"
  etag               = filemd5("${path.module}/packages/example-app-1.0.0.pkg")
  checksum_algorithm = "SHA256" # makes HeadObject return ChecksumSHA256 (FULL_OBJECT)
}

resource "fleetdm_software_package" "example_app_s3" {
  team_id  = fleetdm_team.workstations.id
  filename = "example-app-1.0.0.pkg"

  package_s3 = {
    bucket = aws_s3_object.example_app.bucket
    key    = aws_s3_object.example_app.key
    region = "eu-west-1" # optional, uses AWS_REGION if omitted
  }

  install_script = "installer -pkg $INSTALLER_PATH -target /"
  self_service   = true
}

# Variant: bucket is read-only to your runner, so you can't set a SHA256 on the
# object itself. Use package_s3.expected_sha256 to assert the SHA out-of-band.
# WARNING: you are responsible for keeping this value in sync with the actual
# object — if it's wrong, Fleet will think the installer is unchanged and will
# NOT be re-uploaded.

resource "fleetdm_software_package" "example_app_s3_pinned" {
  team_id  = fleetdm_team.workstations.id
  filename = "example-app-1.0.0.pkg"

  package_s3 = {
    bucket          = "vendor-bucket-readonly"
    key             = "installers/example-app-1.0.0.pkg"
    region          = "eu-west-1"
    expected_sha256 = filesha256("${path.module}/packages/example-app-1.0.0.pkg")
  }

  install_script = "installer -pkg $INSTALLER_PATH -target /"
  self_service   = true
}

# --- type = "vpp" ---
# Add an App Store (VPP) app to a team.
# Requires VPP to be configured in Fleet.

data "fleetdm_app_store_apps" "available" {
  team_id = fleetdm_team.workstations.id
}

resource "fleetdm_software_package" "xcode" {
  type         = "vpp"
  app_store_id = "497799835" # Xcode
  team_id      = fleetdm_team.workstations.id
  platform     = "darwin"
  self_service = false
}

# --- type = "fleet_maintained" ---
# Add a Fleet Maintained App (pre-packaged by Fleet) to a team.

data "fleetdm_fleet_maintained_app" "chrome" {
  name = "Google Chrome"
}

resource "fleetdm_software_package" "chrome" {
  type                    = "fleet_maintained"
  fleet_maintained_app_id = data.fleetdm_fleet_maintained_app.chrome.id
  team_id                 = fleetdm_team.workstations.id
  self_service            = true
}

# Fleet Maintained App with a custom install script override
resource "fleetdm_software_package" "chrome_custom" {
  type                    = "fleet_maintained"
  fleet_maintained_app_id = data.fleetdm_fleet_maintained_app.chrome.id
  team_id                 = fleetdm_team.workstations.id

  install_script = data.fleetdm_fleet_maintained_app.chrome.install_script

  self_service      = true
  automatic_install = true
}
```

## SHA256 verification with S3 sources

When `package_s3` is set, the provider tries to learn the S3 object's SHA256
**without downloading the body** so that an unchanged installer becomes a true
no-op on `terraform apply`. This matters for large installers and metered CI
egress: the headline case is a single `HeadObject` call + a single
`GET /software/titles/{id}` against Fleet, and zero bytes of installer transfer.

**S3 does not produce a SHA256 by default.** The default object ETag is the MD5
of the body (for single-part uploads) or a non-SHA hash-of-hashes (for
multipart). Neither matches `sha256(content)`, so the provider cannot rely on
the ETag. You must opt in to one of the following — listed in the order the
provider checks them:

1. **`package_s3.expected_sha256`** (this resource). Lowercase hex SHA256 you
   compute and pin in your Terraform config. The provider trusts this value and
   skips `HeadObject` entirely. Use this when you cannot modify the S3 object
   (e.g. read-only vendor bucket). **You are responsible for keeping this
   accurate** — if it disagrees with the actual object, Fleet will treat the
   installer as unchanged and will not re-upload.

2. **Server-managed SHA256** (`ChecksumSHA256` with `ChecksumType=FULL_OBJECT`).
   The cleanest option when you control the upload. With AWS CLI:

   ```bash
   aws s3api put-object \
     --bucket my-bucket \
     --key installers/app.pkg \
     --body app.pkg \
     --checksum-algorithm SHA256
   ```

   With the Terraform AWS provider, set `checksum_algorithm = "SHA256"` on
   `aws_s3_object` (see the example above).

3. **Object metadata** (`x-amz-meta-sha256`). Lowercase hex SHA256 stored as a
   user metadata header. Useful when you need multipart uploads:

   ```bash
   aws s3 cp app.pkg s3://my-bucket/installers/app.pkg \
     --metadata sha256=$(sha256sum app.pkg | cut -d' ' -f1)
   ```

**Composite multipart checksums are NOT supported.** S3's multipart
`ChecksumSHA256` with `ChecksumType=COMPOSITE` is computed as
`sha256(concat(part-sha256s))` — it does *not* equal `sha256(content)` and
therefore cannot be compared to the SHA Fleet stores. If the provider sees
this, it fails with an actionable error message listing all three remediation
paths above.

**Safe fallback when no SHA256 is available.** If none of the three sources is
present, the provider falls back to downloading the body on every apply and
hashing it locally — the same behavior as before this optimization existed. A
warning explains how to opt into the fast path. The download fallback never
silently re-uploads when content is unchanged: it computes the local SHA and
compares to Fleet's stored hash, exactly as today.

**`package_path` (local file) is always fast.** Hashing a local file is cheap,
so the local-file source has always been a no-op when content is unchanged.

<!-- schema generated by tfplugindocs -->
## Schema

### Optional

- `app_store_id` (String) The App Store ID (Adam ID) for VPP apps. Required when type is 'vpp'.
- `automatic_install` (Boolean) Whether to automatically install the software during device setup (install during setup). Defaults to false.
- `filename` (String) The filename of the package (e.g., 'myapp-1.0.0.pkg'). Required for type 'package'.
- `fleet_maintained_app_id` (Number) The Fleet Maintained App ID. Required when type is 'fleet_maintained'.
- `install_script` (String) The script to run during installation. Optional. Used by type 'package' and 'fleet_maintained'.
- `labels_exclude_any` (List of String) List of label names. The software will not be available for hosts that match any of these labels.
- `labels_include_any` (List of String) List of label names. The software will be available for hosts that match any of these labels.
- `package_path` (String) The filesystem path to the software package file. If set, the file will be uploaded to Fleet when its SHA256 differs from the current package. Supports .pkg, .msi, .deb, .rpm, and .exe files. Mutually exclusive with package_s3.
- `package_s3` (Attributes) S3 source for the software package. Alternative to package_path. The provider downloads the object from S3 and uploads it to Fleet. Mutually exclusive with package_path. Note: bucket and key must be known at plan time (they cannot reference computed values from resources that haven't been created yet). (see [below for nested schema](#nestedatt--package_s3))
- `package_sha256` (String) The SHA256 hash of the package in Fleet. Computed from the local file (package_path) or S3 object (package_s3) on create/update, or read from Fleet API. Can be set explicitly to avoid drift on import.
- `platform` (String) The platform (darwin, windows, linux, ipados, ios). Computed for packages, optional for VPP apps.
- `post_install_script` (String) The script to run after installation. Optional.
- `pre_install_query` (String) An osquery SQL query to run before installation. Installation proceeds only if the query returns results. Optional.
- `self_service` (Boolean) Whether the software is available for self-service installation by end users. Defaults to false.
- `team_id` (Number) The ID of the team this software package belongs to. Required for Fleet Premium.
- `type` (String) The type of software to manage. One of: `package` (default) — upload a local installer file (.pkg, .msi, .deb, .rpm, .exe); `vpp` — add an App Store app via Apple Volume Purchase Program, requires `app_store_id`; `fleet_maintained` — add a Fleet-curated app, requires `fleet_maintained_app_id`. Changing this value forces a new resource.
- `uninstall_script` (String) The script to run during uninstallation. Optional. Used by type 'package'.

### Read-Only

- `id` (Number) The unique identifier (internal, same as title_id).
- `name` (String) The name of the software (extracted from the package or App Store).
- `title_id` (Number) The software title ID.
- `version` (String) The version of the software.

<a id="nestedatt--package_s3"></a>
### Nested Schema for `package_s3`

Required:

- `bucket` (String) The S3 bucket name.
- `key` (String) The S3 object key.

Optional:

- `endpoint_url` (String) Custom S3 endpoint URL. Useful for S3-compatible services like LocalStack or MinIO.
- `expected_sha256` (String) Lowercase hex SHA256 of the S3 object's content, asserted out-of-band. When set, the provider skips HeadObject and trusts this value as the remote SHA. Use this when the bucket is read-only to your runner and you cannot add a SHA256 checksum or `x-amz-meta-sha256` metadata to the object. You are responsible for keeping this value in sync with the actual object — if it's wrong, Fleet will think the installer is unchanged and the package will NOT be re-uploaded. See the 'SHA256 verification with S3 sources' section of the documentation.
- `region` (String) The AWS region. Uses AWS_REGION or default config if omitted.
