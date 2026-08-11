# Get the self-service categories on a fleet, including the defaults Fleet seeds
data "fleetdm_software_self_service_categories" "workstations" {
  fleet_id = 3
}

# Output all category names
output "category_names" {
  value = [for category in data.fleetdm_software_self_service_categories.workstations.categories : category.name]
}

# Look up a single category ID by name
output "browsers_category_id" {
  value = one([
    for category in data.fleetdm_software_self_service_categories.workstations.categories :
    category.id if category.name == "🌎 Browsers"
  ])
}
