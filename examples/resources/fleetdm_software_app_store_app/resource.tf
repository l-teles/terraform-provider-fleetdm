# Add an App Store (VPP) app to a team. Requires VPP to be configured in Fleet.

data "fleetdm_app_store_apps" "available" {
  team_id = fleetdm_team.workstations.id
}

resource "fleetdm_software_app_store_app" "xcode" {
  app_store_id = "497799835" # Xcode
  team_id      = fleetdm_team.workstations.id
  platform     = "darwin"
  self_service = false
}

# VPP app with label targeting.
resource "fleetdm_software_app_store_app" "design_tools" {
  app_store_id       = "409183694" # Keynote
  team_id            = fleetdm_team.designers.id
  platform           = "darwin"
  self_service       = true
  labels_include_any = ["Designers"]
}
