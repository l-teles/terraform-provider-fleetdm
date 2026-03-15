# Example: Using the mdm_summary data source

# Get overall MDM enrollment summary
data "fleetdm_mdm_summary" "all" {}

# Get MDM summary for macOS devices only
data "fleetdm_mdm_summary" "macos" {
  platform = "darwin"
}

# Get MDM summary for Windows devices only
data "fleetdm_mdm_summary" "windows" {
  platform = "windows"
}

# Get MDM summary for a specific team
data "fleetdm_mdm_summary" "team" {
  team_id = fleetdm_team.engineering.id
}

# Output enrollment statistics
output "total_enrolled" {
  description = "Total enrolled hosts"
  value       = data.fleetdm_mdm_summary.all.enrolled_manual_hosts_count + data.fleetdm_mdm_summary.all.enrolled_automated_hosts_count
}

output "enrollment_breakdown" {
  description = "MDM enrollment breakdown"
  value = {
    manual     = data.fleetdm_mdm_summary.all.enrolled_manual_hosts_count
    automated  = data.fleetdm_mdm_summary.all.enrolled_automated_hosts_count
    personal   = data.fleetdm_mdm_summary.all.enrolled_personal_hosts_count
    pending    = data.fleetdm_mdm_summary.all.pending_hosts_count
    unenrolled = data.fleetdm_mdm_summary.all.unenrolled_hosts_count
    total      = data.fleetdm_mdm_summary.all.hosts_count
  }
}

output "mdm_solutions" {
  description = "MDM solutions in use"
  value = [for s in data.fleetdm_mdm_summary.all.mdm_solutions : {
    name        = s.name
    server_url  = s.server_url
    hosts_count = s.hosts_count
  }]
}

output "macos_enrollment" {
  description = "macOS MDM enrollment count"
  value       = data.fleetdm_mdm_summary.macos.hosts_count
}

output "windows_enrollment" {
  description = "Windows MDM enrollment count"
  value       = data.fleetdm_mdm_summary.windows.hosts_count
}
