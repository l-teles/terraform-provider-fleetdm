# Import a fleet's patch deadline settings using the fleet ID
terraform import fleetdm_software_patch_deadline_settings.workstations 3

# Hosts that are not assigned to a fleet use fleet ID 0
terraform import fleetdm_software_patch_deadline_settings.unassigned 0
