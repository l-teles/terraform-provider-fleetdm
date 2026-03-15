# Example: Listing FleetDM users

# Get all users
data "fleetdm_users" "all" {}

# Filter users by query
data "fleetdm_users" "admins" {
  query = "admin"
}

# Output user information
output "total_users" {
  description = "Total number of users"
  value       = length(data.fleetdm_users.all.users)
}

output "user_list" {
  description = "List of all users"
  value = [for u in data.fleetdm_users.all.users : {
    id    = u.id
    name  = u.name
    email = u.email
    role  = u.global_role
  }]
}

output "admin_users" {
  description = "Users matching 'admin' query"
  value = [for u in data.fleetdm_users.admins.users : {
    name  = u.name
    email = u.email
  }]
}

# Find users with global admin role
output "global_admins" {
  description = "Users with global admin role"
  value       = [for u in data.fleetdm_users.all.users : u if u.global_role == "admin"]
}

# Find API-only users
output "api_users" {
  description = "API-only users (automation accounts)"
  value       = [for u in data.fleetdm_users.all.users : u if u.api_only == true]
}

# Find SSO-enabled users
output "sso_users" {
  description = "Users with SSO enabled"
  value       = [for u in data.fleetdm_users.all.users : u.email if u.sso_enabled == true]
}
