# Get Fleet server version information
data "fleetdm_version" "current" {}

output "fleet_version" {
  value = data.fleetdm_version.current.version
}

output "fleet_branch" {
  value = data.fleetdm_version.current.branch
}

output "fleet_revision" {
  value = data.fleetdm_version.current.revision
}

output "go_version" {
  value = data.fleetdm_version.current.go_version
}

# Example: Use version in a local for conditional logic
locals {
  fleet_major_version = split(".", data.fleetdm_version.current.version)[0]
  is_premium          = contains(["premium", "ultimate"], "premium") # Check license tier via configuration
}
