# Add a Fleet Maintained App (a Fleet-curated installer recipe) to a team.

data "fleetdm_fleet_maintained_app" "chrome" {
  name = "Google Chrome"
}

# Fleet keeps ownership of install_script and uninstall_script here: they are
# not declared, so Fleet's own scripts apply and Fleet keeps them current as it
# publishes new versions of the app. Terraform stores no copy and plans no
# change when they move upstream.
resource "fleetdm_software_fleet_maintained_app" "chrome" {
  fleet_maintained_app_id = data.fleetdm_fleet_maintained_app.chrome.id
  team_id                 = fleetdm_fleet.workstations.id
  self_service            = true
}

# Declaring a script takes ownership of it: the configured value becomes
# authoritative, edits made in the Fleet UI are reverted on the next apply, and
# Fleet's upstream updates to that script no longer apply. Remove the attribute
# to hand ownership back to Fleet without clearing the script.
resource "fleetdm_software_fleet_maintained_app" "chrome_custom" {
  fleet_maintained_app_id = data.fleetdm_fleet_maintained_app.chrome.id
  team_id                 = fleetdm_fleet.kiosks.id

  install_script = <<-EOT
    #!/bin/sh
    installer -pkg "$INSTALLER_PATH" -target /
    defaults write com.google.Chrome KioskMode -bool true
  EOT

  self_service             = true
  automatic_install_policy = true
}
