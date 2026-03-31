# Get all reports
data "fleetdm_reports" "all" {}

# Output all report names
output "all_report_names" {
  value = [for report in data.fleetdm_reports.all.reports : report.name]
}

# Get reports for a specific fleet
data "fleetdm_reports" "fleet_reports" {
  fleet_id = fleetdm_fleet.workstations.id
}

# Find scheduled reports
output "scheduled_reports" {
  value = [for report in data.fleetdm_reports.all.reports : report.name if report.interval > 0]
}
