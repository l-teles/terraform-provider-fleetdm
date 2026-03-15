# Get a specific query by ID
data "fleetdm_query" "os_version" {
  id = 1
}

# Use query data
output "query_name" {
  value = data.fleetdm_query.os_version.name
}

output "query_sql" {
  value = data.fleetdm_query.os_version.query
}

output "query_interval" {
  value = data.fleetdm_query.os_version.interval
}
