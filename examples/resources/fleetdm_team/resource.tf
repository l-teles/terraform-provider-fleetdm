# Create a basic team
resource "fleetdm_team" "workstations" {
  name        = "Workstations"
  description = "All workstation devices"
}

# Create a team with host expiry settings
resource "fleetdm_team" "servers" {
  name        = "Servers"
  description = "Production servers"

  host_expiry_enabled = true
  host_expiry_window  = 30 # Days
}

# Create a team with disk encryption enabled
resource "fleetdm_team" "secure_workstations" {
  name        = "Secure Workstations"
  description = "Workstations with enhanced security"

  enable_disk_encryption = true
  host_expiry_enabled    = true
  host_expiry_window     = 14
}
