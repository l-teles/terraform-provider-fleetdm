# Example: FleetDM Setup Experience

# Configure setup experience for a team
resource "fleetdm_setup_experience" "workstations" {
  team_id = fleetdm_team.workstations.id

  # Require end user authentication during device setup
  enable_end_user_authentication = true

  # Require admin to manually release the device after setup
  enable_release_device_manually = false
}

# Setup experience with manual device release
resource "fleetdm_setup_experience" "engineering" {
  team_id = fleetdm_team.engineering.id

  # Enable both authentication and manual release
  enable_end_user_authentication = true
  enable_release_device_manually = true
}

# Default setup experience (no authentication required)
resource "fleetdm_setup_experience" "contractors" {
  team_id = fleetdm_team.contractors.id

  enable_end_user_authentication = false
  enable_release_device_manually = false
}
