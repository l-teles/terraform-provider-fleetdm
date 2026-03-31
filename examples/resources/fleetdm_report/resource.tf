# Create a simple report
resource "fleetdm_report" "os_version" {
  name        = "Get OS Version"
  description = "Returns the operating system version for each host"
  query       = "SELECT * FROM os_version"
  platform    = ["darwin", "linux", "windows"]
}

# Create a scheduled report
resource "fleetdm_report" "disk_usage" {
  name        = "Disk Usage"
  description = "Monitor disk usage every hour"
  query       = "SELECT * FROM disk_info"
  interval    = 3600 # Run every hour
  logging     = "snapshot"
}

# Create a report for a specific fleet
resource "fleetdm_report" "fleet_report" {
  name        = "Fleet Specific Report"
  description = "A report only for a specific fleet"
  query       = "SELECT * FROM users"
  fleet_id    = fleetdm_fleet.workstations.id

  observer_can_run = true
}

# Create a report with all options
resource "fleetdm_report" "comprehensive" {
  name                = "Comprehensive Report"
  description         = "A report with all options configured"
  query               = "SELECT * FROM system_info"
  platform            = ["darwin"]
  min_osquery_version = "5.0.0"
  interval            = 300
  observer_can_run    = true
  automations_enabled = false
  logging             = "differential"
  discard_data        = false
}
