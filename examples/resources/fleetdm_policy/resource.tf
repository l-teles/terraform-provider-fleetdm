# Create a global policy for disk encryption
resource "fleetdm_policy" "disk_encryption" {
  name        = "Disk Encryption Enabled"
  description = "Verifies that disk encryption is enabled on the host"
  query       = "SELECT 1 FROM disk_encryption WHERE encrypted = 1"
  critical    = true
  resolution  = "Enable FileVault on macOS or BitLocker on Windows"
  platform    = ["darwin", "windows"]
}

# Create a policy for firewall status
resource "fleetdm_policy" "firewall_enabled" {
  name        = "Firewall Enabled"
  description = "Verifies that the firewall is enabled"
  query       = "SELECT 1 FROM alf WHERE global_state >= 1"
  resolution  = "Enable the firewall in System Preferences > Security & Privacy > Firewall"
  platform    = ["darwin"]
}

# Create a team-specific policy
resource "fleetdm_policy" "team_policy" {
  name        = "Team Security Check"
  description = "A team-specific security policy"
  query       = "SELECT 1 FROM os_version WHERE major >= 12"
  team_id     = fleetdm_fleet.workstations.id
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

# Team policy with a run-script automation: when the policy fails on a host,
# Fleet runs the linked script to remediate.
resource "fleetdm_policy" "auto_remediate" {
  name      = "Gatekeeper Enabled"
  query     = "SELECT 1 FROM gatekeeper WHERE assessments_enabled = 1;"
  team_id   = fleetdm_fleet.workstations.id
  platform  = ["darwin"]
  script_id = fleetdm_script.enable_gatekeeper.id

  # Restrict the policy to hosts that carry any of these labels.
  labels_include_any = ["Macs on Sonoma"]
}

# Patch policy: tied to a Fleet-maintained software title and automatically
# updated as new versions ship. type and patch_software_title_id are
# immutable after create. Note that `query` must be omitted — Fleet
# generates the query from the linked software title.
resource "fleetdm_policy" "patch_acrobat" {
  name    = "Adobe Acrobat up to date"
  team_id = fleetdm_fleet.workstations.id
  type    = "patch"
  # The fleetdm_software_title data source exposes id as a string;
  # patch_software_title_id expects a number, so cast with tonumber.
  patch_software_title_id = tonumber(data.fleetdm_software_title.adobe_acrobat.id)
}

# Team policy with conditional access: failing hosts are blocked from SSO
# until they remediate. Only applies to team policies.
resource "fleetdm_policy" "conditional_access" {
  name                       = "Disk encryption required for SSO"
  query                      = "SELECT 1 FROM disk_encryption WHERE encrypted = 1;"
  team_id                    = fleetdm_fleet.workstations.id
  conditional_access_enabled = true
  calendar_events_enabled    = true
}
