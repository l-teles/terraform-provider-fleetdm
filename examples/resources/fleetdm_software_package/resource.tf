# Example: FleetDM Software Package

# Upload a software package for a team
resource "fleetdm_software_package" "example_app" {
  team_id = fleetdm_team.workstations.id

  filename     = "example-app-1.0.0.pkg"
  package_path = "${path.module}/packages/example-app-1.0.0.pkg"

  install_script      = "installer -pkg /tmp/example-app-1.0.0.pkg -target /"
  uninstall_script    = "rm -rf /Applications/ExampleApp.app"
  post_install_script = "open /Applications/ExampleApp.app"

  self_service      = true
  automatic_install = false
}

# Software package with pre-install query
resource "fleetdm_software_package" "conditional_app" {
  team_id = fleetdm_team.workstations.id

  filename     = "conditional-app.pkg"
  package_path = "${path.module}/packages/conditional-app.pkg"

  # Only install if the app is not already installed
  pre_install_query = "SELECT 1 FROM apps WHERE name != 'ConditionalApp';"

  install_script = "installer -pkg /tmp/conditional-app.pkg -target /"
}

# Software package with label targeting
resource "fleetdm_software_package" "developer_tools" {
  team_id = fleetdm_team.engineering.id

  filename     = "dev-tools.pkg"
  package_path = "${path.module}/packages/dev-tools.pkg"

  install_script = "installer -pkg /tmp/dev-tools.pkg -target /"

  # Only available for hosts with these labels
  labels_include_any = ["Developers", "Engineers"]

  # Exclude hosts with this label
  labels_exclude_any = ["Contractors"]

  self_service = true
}
