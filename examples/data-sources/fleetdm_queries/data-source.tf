# Get all queries
data "fleetdm_queries" "all" {}

# Output all query names
output "all_query_names" {
  value = [for query in data.fleetdm_queries.all.queries : query.name]
}

# Get queries for a specific team
data "fleetdm_queries" "team_queries" {
  team_id = fleetdm_team.workstations.id
}

# Find scheduled queries
output "scheduled_queries" {
  value = [for query in data.fleetdm_queries.all.queries : query.name if query.interval > 0]
}
