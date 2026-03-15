# Create a global policy for disk encryption
resource "fleetdm_policy" "disk_encryption" {
  name        = "Disk Encryption Enabled"
  description = "Verifies that disk encryption is enabled on the host"
  query       = "SELECT 1 FROM disk_encryption WHERE encrypted = 1"
  critical    = true
  resolution  = "Enable FileVault on macOS or BitLocker on Windows"
  platform    = "darwin,windows"
}

# Create a policy for firewall status
resource "fleetdm_policy" "firewall_enabled" {
  name        = "Firewall Enabled"
  description = "Verifies that the firewall is enabled"
  query       = "SELECT 1 FROM alf WHERE global_state >= 1"
  resolution  = "Enable the firewall in System Preferences > Security & Privacy > Firewall"
  platform    = "darwin"
}

# Create a team-specific policy
resource "fleetdm_policy" "team_policy" {
  name        = "Team Security Check"
  description = "A team-specific security policy"
  query       = "SELECT 1 FROM os_version WHERE major >= 12"
  team_id     = fleetdm_team.workstations.id
  critical    = false
  resolution  = "Update to the latest macOS version"
}

# Create a critical policy for password complexity
resource "fleetdm_policy" "password_policy" {
  name        = "Strong Password Policy"
  description = "Verifies strong password requirements are configured"
  query       = "SELECT 1 FROM password_policy WHERE min_length >= 12"
  critical    = true
  resolution  = "Configure password policy to require at least 12 characters"
}
