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

# Look up a Fleet Maintained App by ID, scoped to a team.
data "fleetdm_fleet_maintained_app" "by_id" {
  id      = 3
  team_id = fleetdm_team.workstations.id
}

# Disambiguate an app Fleet publishes under the same name on several platforms.
# Without platform, a name matching more than one platform is an error rather
# than a guess at which one you meant.
data "fleetdm_fleet_maintained_app" "firefox_windows" {
  name     = "Mozilla Firefox"
  platform = "windows"
}

# name/platform set alongside id must match the resolved app, or the read
# errors — they're a consistency check here, not a second lookup key.
data "fleetdm_fleet_maintained_app" "firefox_by_id" {
  id       = 93926
  name     = "Mozilla Firefox"
  platform = "windows"
}

# Use the app ID to add it to a team via fleetdm_software_fleet_maintained_app.
resource "fleetdm_software_fleet_maintained_app" "chrome" {
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
