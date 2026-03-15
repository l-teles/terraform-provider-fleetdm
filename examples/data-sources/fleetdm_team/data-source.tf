# Get a specific team by ID
data "fleetdm_team" "workstations" {
  id = 1
}

# Use team data in other resources
output "team_name" {
  value = data.fleetdm_team.workstations.name
}

output "team_host_count" {
  value = data.fleetdm_team.workstations.host_count
}
