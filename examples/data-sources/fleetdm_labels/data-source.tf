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
