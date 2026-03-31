# Get a specific fleet by ID
data "fleetdm_fleet" "workstations" {
  id = 1
}

# Use fleet data in other resources
output "fleet_name" {
  value = data.fleetdm_fleet.workstations.name
}

output "fleet_host_count" {
  value = data.fleetdm_fleet.workstations.host_count
}
