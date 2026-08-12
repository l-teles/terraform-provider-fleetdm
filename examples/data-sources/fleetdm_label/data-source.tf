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

# How the label selects hosts: "dynamic", "host_vitals" or "manual"
output "label_membership_type" {
  value = data.fleetdm_label.macos.label_membership_type
}

# For a host vitals label, the attribute comparison driving membership.
# Null for dynamic and manual labels.
output "label_criteria" {
  value = data.fleetdm_label.macos.criteria
}
