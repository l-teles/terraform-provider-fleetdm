# Example: Managing FleetDM users

# Create a global admin user
resource "fleetdm_user" "admin" {
  name        = "Admin User"
  email       = "admin@example.com"
  password    = var.admin_password
  global_role = "admin"
}

# Create an observer user with read-only access
resource "fleetdm_user" "observer" {
  name        = "Security Observer"
  email       = "observer@example.com"
  password    = var.observer_password
  global_role = "observer"
}

# Create a maintainer user
resource "fleetdm_user" "maintainer" {
  name        = "Fleet Maintainer"
  email       = "maintainer@example.com"
  password    = var.maintainer_password
  global_role = "maintainer"
}

# Create an API-only user (for automation)
resource "fleetdm_user" "api_user" {
  name        = "CI/CD Automation"
  email       = "automation@example.com"
  password    = var.api_password
  global_role = "observer_plus"
  api_only    = true
}

# Create a team-specific user (no global role, assigned to teams)
resource "fleetdm_user" "team_admin" {
  name     = "Engineering Team Admin"
  email    = "team-admin@example.com"
  password = var.team_admin_password

  teams = [
    {
      id   = fleetdm_team.engineering.id
      role = "admin"
    },
    {
      id   = fleetdm_team.security.id
      role = "observer"
    }
  ]
}

# Variables for passwords (should be provided securely)
variable "admin_password" {
  type      = string
  sensitive = true
}

variable "observer_password" {
  type      = string
  sensitive = true
}

variable "maintainer_password" {
  type      = string
  sensitive = true
}

variable "api_password" {
  type      = string
  sensitive = true
}

variable "team_admin_password" {
  type      = string
  sensitive = true
}

# Output user IDs
output "admin_user_id" {
  description = "ID of the admin user"
  value       = fleetdm_user.admin.id
}

output "api_user_id" {
  description = "ID of the API automation user"
  value       = fleetdm_user.api_user.id
}
