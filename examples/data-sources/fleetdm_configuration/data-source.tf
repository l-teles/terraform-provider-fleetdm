# Get Fleet server configuration
data "fleetdm_configuration" "current" {}

output "org_name" {
  value = data.fleetdm_configuration.current.org_name
}

output "server_url" {
  value = data.fleetdm_configuration.current.server_url
}

output "license_tier" {
  value = data.fleetdm_configuration.current.license_tier
}

output "license_expiration" {
  value = data.fleetdm_configuration.current.license_expiration
}

# Example: Conditional resources based on license
locals {
  is_premium = data.fleetdm_configuration.current.license_tier == "premium"
}

# Only create team resources if premium license
resource "fleetdm_team" "example" {
  count       = local.is_premium ? 1 : 0
  name        = "Premium Team"
  description = "This team requires Fleet Premium"
}
