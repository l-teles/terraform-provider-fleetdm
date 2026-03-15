# =============================================================================
# Outputs - Display created resources
# =============================================================================

# -----------------------------------------------------------------------------
# Server Info
# -----------------------------------------------------------------------------

output "fleetdm_version" {
  description = "FleetDM server version"
  value       = data.fleetdm_version.current.version
}

output "existing_teams_count" {
  description = "Number of existing teams"
  value       = length(data.fleetdm_teams.all.teams)
}

# -----------------------------------------------------------------------------
# Created Resources
# -----------------------------------------------------------------------------

output "test_team" {
  description = "Test team details"
  value = {
    id   = fleetdm_team.test.id
    name = fleetdm_team.test.name
  }
}

output "labels" {
  description = "Created labels"
  value = {
    macos     = fleetdm_label.macos.id
    all_hosts = fleetdm_label.all_hosts.id
  }
}

output "queries" {
  description = "Created queries"
  value = {
    system_info = fleetdm_query.system_info.id
    os_version  = fleetdm_query.os_version.id
  }
}

output "policies" {
  description = "Created policies"
  value = {
    disk_encryption  = fleetdm_policy.disk_encryption.id
    screensaver_lock = fleetdm_policy.screensaver_lock.id
  }
}

output "scripts" {
  description = "Created scripts"
  value = {
    hello_world  = fleetdm_script.hello_world.id
    system_check = fleetdm_script.system_check.id
  }
}

output "software_package" {
  description = "Crowdstrike software package"
  value = {
    id       = fleetdm_software_package.crowdstrike.id
    title_id = fleetdm_software_package.crowdstrike.title_id
    name     = fleetdm_software_package.crowdstrike.name
    version  = fleetdm_software_package.crowdstrike.version
    platform = fleetdm_software_package.crowdstrike.platform
  }
}

output "enroll_secret" {
  description = "Team enroll secret (use this to enroll hosts to the test team)"
  value       = fleetdm_enroll_secret.test_team.secrets[0].secret
  sensitive   = true
}

# -----------------------------------------------------------------------------
# Summary
# -----------------------------------------------------------------------------

output "summary" {
  description = "Test infrastructure summary"
  value       = <<-EOT

    ============================================
    FleetDM Terraform Provider Test - Complete!
    ============================================

    Server Version: ${data.fleetdm_version.current.version}

    Created Resources:
      - Team: ${fleetdm_team.test.name} (ID: ${fleetdm_team.test.id})
      - Labels: 2
      - Queries: 2  
      - Policies: 2
      - Scripts: 2
      - Software Package: ${fleetdm_software_package.crowdstrike.name}

    To clean up, run:
      terraform destroy

    ============================================
  EOT
}
