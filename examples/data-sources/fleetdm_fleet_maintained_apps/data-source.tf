# List all Fleet Maintained Apps
data "fleetdm_fleet_maintained_apps" "all" {}

# List Fleet Maintained Apps, annotated for a specific team
# (software_title_id is populated for apps already added to that team)
data "fleetdm_fleet_maintained_apps" "team" {
  team_id = fleetdm_team.workstations.id
}

output "available_app_count" {
  value = length(data.fleetdm_fleet_maintained_apps.all.fleet_maintained_apps)
}

output "app_names" {
  value = [for app in data.fleetdm_fleet_maintained_apps.all.fleet_maintained_apps : app.name]
}
