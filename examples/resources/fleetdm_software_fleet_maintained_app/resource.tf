# Add a Fleet Maintained App (a Fleet-curated installer recipe) to a team.

data "fleetdm_fleet_maintained_app" "chrome" {
  name = "Google Chrome"
}

resource "fleetdm_software_fleet_maintained_app" "chrome" {
  fleet_maintained_app_id = data.fleetdm_fleet_maintained_app.chrome.id
  team_id                 = fleetdm_team.workstations.id
  self_service            = true
}

# Fleet Maintained App with a custom install script override.
resource "fleetdm_software_fleet_maintained_app" "chrome_custom" {
  fleet_maintained_app_id = data.fleetdm_fleet_maintained_app.chrome.id
  team_id                 = fleetdm_team.workstations.id

  install_script = data.fleetdm_fleet_maintained_app.chrome.install_script

  self_service      = true
  automatic_install = true
}
