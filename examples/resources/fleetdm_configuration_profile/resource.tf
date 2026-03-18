# Example: Managing MDM configuration profiles

# Create a macOS configuration profile from a .mobileconfig file
resource "fleetdm_configuration_profile" "wifi_settings" {
  team_id         = 0 # 0 for global, or use a team ID
  profile_content = file("${path.module}/profiles/wifi-settings.mobileconfig")
}

# Create a Windows configuration profile with a display name
resource "fleetdm_configuration_profile" "windows_bitlocker" {
  team_id         = fleetdm_team.workstations.id
  display_name    = "BitLocker Policy"
  profile_content = file("${path.module}/profiles/bitlocker-policy.xml")
}

# Create a profile with label targeting
resource "fleetdm_configuration_profile" "secure_dns" {
  team_id            = 0
  profile_content    = file("${path.module}/profiles/secure-dns.mobileconfig")
  labels_include_all = ["Production", "Managed"]
}

# Create a profile excluding certain labels
resource "fleetdm_configuration_profile" "advanced_security" {
  team_id            = fleetdm_team.engineering.id
  profile_content    = file("${path.module}/profiles/advanced-security.mobileconfig")
  labels_include_any = ["HighSecurity"]
  labels_exclude_any = ["Development", "Testing"]
}

# Output profile information
output "wifi_profile_uuid" {
  description = "UUID of the WiFi settings profile"
  value       = fleetdm_configuration_profile.wifi_settings.profile_uuid
}

output "wifi_profile_name" {
  description = "Name extracted from the profile content"
  value       = fleetdm_configuration_profile.wifi_settings.name
}
