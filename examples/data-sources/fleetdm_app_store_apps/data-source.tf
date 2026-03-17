# List available App Store (VPP) apps for a team
data "fleetdm_app_store_apps" "available" {
  team_id = fleetdm_team.workstations.id
}

output "available_vpp_app_count" {
  value = length(data.fleetdm_app_store_apps.available.app_store_apps)
}

output "vpp_app_names" {
  value = [for app in data.fleetdm_app_store_apps.available.app_store_apps : app.name]
}
