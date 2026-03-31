# Get all fleets
data "fleetdm_fleets" "all" {}

# Output all fleet names
output "all_fleet_names" {
  value = [for fleet in data.fleetdm_fleets.all.fleets : fleet.name]
}

# Find fleets with hosts
output "fleets_with_hosts" {
  value = [for fleet in data.fleetdm_fleets.all.fleets : fleet.name if fleet.host_count > 0]
}
