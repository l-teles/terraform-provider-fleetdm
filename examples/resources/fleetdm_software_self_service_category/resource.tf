# Group self-service software on a fleet so end users can browse it by category
resource "fleetdm_fleet" "workstations" {
  name = "Workstations"
}

resource "fleetdm_software_self_service_category" "engineering" {
  fleet_id = fleetdm_fleet.workstations.id
  name     = "💼 Engineering"
}

# Categories for hosts that are not assigned to a fleet use fleet_id 0
resource "fleetdm_software_self_service_category" "unassigned_design" {
  fleet_id = 0
  name     = "🎨 Design"
}
