# Attach a custom icon to a software title, replacing the one Fleet derives
# automatically in Fleet Desktop and the self-service catalog.
#
# The image must be a PNG, between 120x120 and 1024x1024 pixels, and under
# 100KB. Fleet validates the file by decoding it, so the extension is
# irrelevant — a mislabelled JPEG is rejected.

resource "fleetdm_software_custom_package" "example_app" {
  team_id = fleetdm_fleet.workstations.id

  filename     = "example-app-1.0.0.pkg"
  package_path = "${path.module}/packages/example-app-1.0.0.pkg"

  install_script = "installer -pkg $INSTALLER_PATH -target /"
  self_service   = true
}

resource "fleetdm_software_title_icon" "example_app" {
  title_id  = fleetdm_software_custom_package.example_app.title_id
  fleet_id  = fleetdm_fleet.workstations.id
  icon_path = "${path.module}/icons/example-app.png"
}

# Icons are scoped per fleet: the same title offered on several fleets needs one
# resource per fleet, and each can carry a different image.

resource "fleetdm_software_title_icon" "example_app_engineering" {
  title_id  = fleetdm_software_custom_package.example_app.title_id
  fleet_id  = fleetdm_fleet.engineering.id
  icon_path = "${path.module}/icons/example-app-engineering.png"
}

# Use fleet_id 0 for the "No team" fleet.

resource "fleetdm_software_title_icon" "shared_tool" {
  title_id  = data.fleetdm_software_title.shared_tool.id
  fleet_id  = 0
  icon_path = "${path.module}/icons/shared-tool.png"
}
