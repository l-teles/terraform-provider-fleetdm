# Get all global policies
data "fleetdm_policies" "global" {}

# Get team-specific policies
data "fleetdm_policies" "team" {
  team_id = fleetdm_team.workstations.id
}

# Get only the policies targeting one platform (Fleet 4.90+)
data "fleetdm_policies" "macos" {
  platform = "darwin"
}

# Output all policy names
output "all_policy_names" {
  value = [for policy in data.fleetdm_policies.global.policies : policy.name]
}

# Find critical policies
output "critical_policies" {
  value = [for policy in data.fleetdm_policies.global.policies : policy.name if policy.critical]
}

# Find policies with failures
output "policies_with_failures" {
  value = [for policy in data.fleetdm_policies.global.policies : policy.name if policy.failing_host_count > 0]
}

# Calculate overall compliance
output "total_passing" {
  value = sum([for policy in data.fleetdm_policies.global.policies : policy.passing_host_count])
}
