# Create a simple query
resource "fleetdm_query" "os_version" {
  name        = "Get OS Version"
  description = "Returns the operating system version for each host"
  query       = "SELECT * FROM os_version"
  platform    = "darwin,linux,windows"
}

# Create a scheduled query
resource "fleetdm_query" "disk_usage" {
  name        = "Disk Usage"
  description = "Monitor disk usage every hour"
  query       = "SELECT * FROM disk_info"
  interval    = 3600 # Run every hour
  logging     = "snapshot"
}

# Create a query for a specific team
resource "fleetdm_query" "team_query" {
  name        = "Team Specific Query"
  description = "A query only for a specific team"
  query       = "SELECT * FROM users"
  team_id     = fleetdm_team.workstations.id

  observer_can_run = true
}

# Create a query with all options
resource "fleetdm_query" "comprehensive" {
  name                = "Comprehensive Query"
  description         = "A query with all options configured"
  query               = "SELECT * FROM system_info"
  platform            = "darwin"
  min_osquery_version = "5.0.0"
  interval            = 300
  observer_can_run    = true
  automations_enabled = false
  logging             = "differential"
  discard_data        = false
}
