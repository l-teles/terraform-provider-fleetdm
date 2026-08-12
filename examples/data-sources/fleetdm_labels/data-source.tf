# Get all labels
data "fleetdm_labels" "all" {}

# Output all label names
output "all_label_names" {
  value = [for label in data.fleetdm_labels.all.labels : label.name]
}

# Find labels with hosts
output "labels_with_hosts" {
  value = [for label in data.fleetdm_labels.all.labels : label.name if label.host_count > 0]
}

# Count total hosts across all labels
output "total_labeled_hosts" {
  value = sum([for label in data.fleetdm_labels.all.labels : label.host_count])
}

# Group labels by how they select hosts
output "labels_by_membership_type" {
  value = {
    for label in data.fleetdm_labels.all.labels :
    label.name => label.label_membership_type
  }
}

# Host vitals labels and the attribute comparison behind each one. The list
# endpoint echoes criteria in full, so no extra lookup is needed.
output "host_vitals_labels" {
  value = {
    for label in data.fleetdm_labels.all.labels :
    label.name => label.criteria
    if label.label_membership_type == "host_vitals"
  }
}
