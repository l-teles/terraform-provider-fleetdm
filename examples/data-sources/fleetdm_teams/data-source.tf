# Get all teams
data "fleetdm_teams" "all" {}

# Output all team names
output "all_team_names" {
  value = [for team in data.fleetdm_teams.all.teams : team.name]
}

# Find teams with hosts
output "teams_with_hosts" {
  value = [for team in data.fleetdm_teams.all.teams : team.name if team.host_count > 0]
}
