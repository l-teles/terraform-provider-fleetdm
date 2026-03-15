# List all global scripts
data "fleetdm_scripts" "all" {}

output "script_count" {
  value = length(data.fleetdm_scripts.all.scripts)
}

output "script_names" {
  value = [for s in data.fleetdm_scripts.all.scripts : s.name]
}

# List scripts for a specific team
data "fleetdm_scripts" "team_scripts" {
  team_id = fleetdm_team.workstations.id
}

output "team_script_names" {
  value = [for s in data.fleetdm_scripts.team_scripts.scripts : s.name]
}
