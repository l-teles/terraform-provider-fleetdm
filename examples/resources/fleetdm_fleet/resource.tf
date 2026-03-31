# Create a basic fleet
resource "fleetdm_fleet" "workstations" {
  name        = "Workstations"
  description = "All workstation devices"
}

# Create a fleet with host expiry settings
resource "fleetdm_fleet" "servers" {
  name        = "Servers"
  description = "Production servers"

  host_expiry_enabled = true
  host_expiry_window  = 30 # Days
}

# Create a fleet with disk encryption enabled
resource "fleetdm_fleet" "secure_workstations" {
  name        = "Secure Workstations"
  description = "Workstations with enhanced security"

  enable_disk_encryption = true
  host_expiry_enabled    = true
  host_expiry_window     = 14
}
