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

resource "aws_s3_object" "example_app" {
  bucket = "my-software-bucket"
  key    = "installers/example-app-1.0.0.pkg"
  source = "${path.module}/packages/example-app-1.0.0.pkg"
  etag   = filemd5("${path.module}/packages/example-app-1.0.0.pkg")
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
