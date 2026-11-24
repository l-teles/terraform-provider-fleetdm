# Set when patch deadlines fall for a fleet's hosts (host-local time)
resource "fleetdm_fleet" "workstations" {
  name = "Workstations"
}

resource "fleetdm_software_patch_deadline_settings" "workstations" {
  fleet_id = fleetdm_fleet.workstations.id
  days     = ["monday", "thursday"]
  time     = "09:00"
}

# Hosts that are not assigned to a fleet use fleet_id 0
resource "fleetdm_software_patch_deadline_settings" "unassigned" {
  fleet_id = 0
  days     = ["wednesday"]
  time     = "18:30"
}
