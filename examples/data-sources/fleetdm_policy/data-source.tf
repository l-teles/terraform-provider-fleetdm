# Get a global policy by ID
data "fleetdm_policy" "disk_encryption" {
  id = 1
}

# Get a team policy by ID
data "fleetdm_policy" "team_policy" {
  id      = 5
  team_id = fleetdm_team.workstations.id
}

# Use policy data
output "policy_name" {
  value = data.fleetdm_policy.disk_encryption.name
}

output "passing_hosts" {
  value = data.fleetdm_policy.disk_encryption.passing_host_count
}

output "failing_hosts" {
  value = data.fleetdm_policy.disk_encryption.failing_host_count
}

locals {
  total_hosts = data.fleetdm_policy.disk_encryption.passing_host_count + data.fleetdm_policy.disk_encryption.failing_host_count
}

output "compliance_rate" {
  value = local.total_hosts > 0 ? data.fleetdm_policy.disk_encryption.passing_host_count / local.total_hosts * 100 : 0
}
