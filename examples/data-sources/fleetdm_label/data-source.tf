# Get a specific label by ID
data "fleetdm_label" "macos" {
  id = 1
}

# Use label data
output "label_name" {
  value = data.fleetdm_label.macos.name
}

output "label_query" {
  value = data.fleetdm_label.macos.query
}

output "hosts_in_label" {
  value = data.fleetdm_label.macos.host_count
}
