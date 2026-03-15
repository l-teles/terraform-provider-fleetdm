# Example: Using the configuration_profiles data source

# Get all global configuration profiles
data "fleetdm_configuration_profiles" "global" {}

# Get all configuration profiles for a specific team
data "fleetdm_configuration_profiles" "team_profiles" {
  team_id = fleetdm_team.engineering.id
}

# Output profile information
output "global_profiles" {
  description = "List of global MDM configuration profiles"
  value = [for p in data.fleetdm_configuration_profiles.global.profiles : {
    name     = p.name
    platform = p.platform
    uuid     = p.profile_uuid
  }]
}

output "team_profile_count" {
  description = "Number of profiles for the team"
  value       = length(data.fleetdm_configuration_profiles.team_profiles.profiles)
}

# Filter profiles by platform
output "macos_profiles" {
  description = "macOS configuration profiles"
  value       = [for p in data.fleetdm_configuration_profiles.global.profiles : p if p.platform == "darwin"]
}

output "windows_profiles" {
  description = "Windows configuration profiles"
  value       = [for p in data.fleetdm_configuration_profiles.global.profiles : p if p.platform == "windows"]
}
