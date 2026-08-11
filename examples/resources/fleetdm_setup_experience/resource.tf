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

  # Fleet 4.90+ settings. Omit any of them to leave Fleet's current value alone.
  lock_end_user_info           = true
  require_all_software_macos   = true
  require_all_software_windows = true
}

# Install fleetd from the team's bootstrap package instead of during
# Setup Assistant (Fleet 4.90+). Requires a bootstrap package, and no
# setup experience software or script on the team.
resource "fleetdm_setup_experience" "kiosks" {
  team_id = fleetdm_team.kiosks.id

  manual_agent_install = true
}

# Default setup experience (no authentication required)
resource "fleetdm_setup_experience" "contractors" {
  team_id = fleetdm_team.contractors.id

  enable_end_user_authentication = false
  enable_release_device_manually = false
}
