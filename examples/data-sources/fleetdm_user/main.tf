# Example: Fetching a single FleetDM user

# Get a user by ID
data "fleetdm_user" "by_id" {
  id = 1
}

# Get a user by email
data "fleetdm_user" "by_email" {
  email = "admin@example.com"
}

# Output user information
output "user_name" {
  description = "User's display name"
  value       = data.fleetdm_user.by_id.name
}

output "user_role" {
  description = "User's global role"
  value       = data.fleetdm_user.by_id.global_role
}

output "user_sso_enabled" {
  description = "Whether SSO is enabled for the user"
  value       = data.fleetdm_user.by_email.sso_enabled
}

output "user_teams" {
  description = "Teams the user belongs to"
  value       = data.fleetdm_user.by_email.teams
}
