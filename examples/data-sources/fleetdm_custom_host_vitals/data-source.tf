# Get all custom host vitals
data "fleetdm_custom_host_vitals" "all" {}

# Output all custom host vital names
output "all_custom_host_vital_names" {
  value = [for vital in data.fleetdm_custom_host_vitals.all.custom_host_vitals : vital.name]
}

# Look up a vital that was created outside Terraform, so a label can match on it
locals {
  asset_tag_id = one([
    for vital in data.fleetdm_custom_host_vitals.all.custom_host_vitals : vital.id
    if vital.name == "Asset tag"
  ])
}

resource "fleetdm_label" "leased_hardware" {
  name = "Leased hardware"

  criteria = {
    vital                = "custom_host_vital"
    operator             = "LIKE"
    value                = "LEASE-%"
    custom_host_vital_id = local.asset_tag_id
  }
}

# Build the $FLEET_HOST_VITAL_<id> tokens for every vital, ready to paste into a
# configuration profile, script or software installer
output "custom_host_vital_variables" {
  value = {
    for vital in data.fleetdm_custom_host_vitals.all.custom_host_vitals :
    vital.name => "$FLEET_HOST_VITAL_${vital.id}"
  }
}
