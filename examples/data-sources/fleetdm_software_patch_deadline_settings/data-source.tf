# Read when patch deadlines fall for a fleet
data "fleetdm_software_patch_deadline_settings" "workstations" {
  fleet_id = 3
}

output "patch_deadline" {
  value = "${join(", ", data.fleetdm_software_patch_deadline_settings.workstations.days)} at ${data.fleetdm_software_patch_deadline_settings.workstations.time}"
}
