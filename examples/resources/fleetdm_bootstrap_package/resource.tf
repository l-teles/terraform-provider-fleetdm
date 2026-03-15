# Example: FleetDM Bootstrap Package

# Upload a bootstrap package for a team
# This package will be automatically installed on macOS devices during DEP enrollment
resource "fleetdm_bootstrap_package" "initial_setup" {
  team_id = fleetdm_team.workstations.id

  name            = "bootstrap-setup-1.0.0.pkg"
  package_content = filebase64("${path.module}/packages/bootstrap-setup-1.0.0.pkg")
}

# Bootstrap package for engineering team
resource "fleetdm_bootstrap_package" "eng_bootstrap" {
  team_id = fleetdm_team.engineering.id

  name            = "engineering-bootstrap.pkg"
  package_content = filebase64("${path.module}/packages/engineering-bootstrap.pkg")
}
