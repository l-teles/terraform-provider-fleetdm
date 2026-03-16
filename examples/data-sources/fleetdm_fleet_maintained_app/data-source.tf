# Look up a Fleet Maintained App by name
data "fleetdm_fleet_maintained_app" "chrome" {
  name = "Google Chrome"
}

# Look up a Fleet Maintained App by name, scoped to a team
# (populates software_title_id if the app is already added to that team)
data "fleetdm_fleet_maintained_app" "chrome_team" {
  name    = "Google Chrome"
  team_id = fleetdm_team.workstations.id
}

# Look up a Fleet Maintained App by ID
data "fleetdm_fleet_maintained_app" "by_id" {
  id = 3
}

# Use the app ID to add it to a team via fleetdm_software_package
resource "fleetdm_software_package" "chrome" {
  type                    = "fleet_maintained"
  fleet_maintained_app_id = data.fleetdm_fleet_maintained_app.chrome.id
  team_id                 = fleetdm_team.workstations.id
  self_service            = true
}

output "chrome_platform" {
  value = data.fleetdm_fleet_maintained_app.chrome.platform
}

output "chrome_version" {
  value = data.fleetdm_fleet_maintained_app.chrome.version
}
