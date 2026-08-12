# Import a software title icon using '{title_id}:{fleet_id}'.
# Both parts are required — an icon has no meaning without its fleet scope.
terraform import fleetdm_software_title_icon.example_app 123:5

# Use fleet_id 0 for the "No team" fleet.
terraform import fleetdm_software_title_icon.shared_tool 123:0
