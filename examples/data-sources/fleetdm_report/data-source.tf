# Get a specific report by ID
data "fleetdm_report" "os_version" {
  id = 1
}

# Use report data
output "report_name" {
  value = data.fleetdm_report.os_version.name
}

output "report_sql" {
  value = data.fleetdm_report.os_version.query
}

output "report_interval" {
  value = data.fleetdm_report.os_version.interval
}
