# Upload a software installer from a local file.
# The package is re-uploaded only when the file content (SHA256) changes.

resource "fleetdm_software_custom_package" "example_app" {
  team_id = fleetdm_team.workstations.id

  filename     = "example-app-1.0.0.pkg"
  package_path = "${path.module}/packages/example-app-1.0.0.pkg"

  install_script      = "installer -pkg /tmp/example-app-1.0.0.pkg -target /"
  uninstall_script    = "rm -rf /Applications/ExampleApp.app"
  post_install_script = "open /Applications/ExampleApp.app"

  self_service      = true
  automatic_install = false
}

# Package with a pre-install query and label targeting.
resource "fleetdm_software_custom_package" "developer_tools" {
  team_id = fleetdm_team.engineering.id

  filename     = "dev-tools.pkg"
  package_path = "${path.module}/packages/dev-tools.pkg"

  # Only install if the tool is not already present.
  pre_install_query = "SELECT 1 FROM apps WHERE name != 'DevTools';"

  install_script = "installer -pkg /tmp/dev-tools.pkg -target /"

  labels_include_any = ["Developers", "Engineers"]

  self_service = true
}

# --- S3 source variants ---
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

resource "fleetdm_software_custom_package" "example_app_s3" {
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

resource "fleetdm_software_custom_package" "example_app_s3_pinned" {
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
