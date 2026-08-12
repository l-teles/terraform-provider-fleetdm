# List every certificate authority configured in Fleet.
# Fleet's list endpoint returns identity only, so id, name and type are all
# that is available — never configuration and never secrets.
data "fleetdm_certificate_authorities" "all" {}

output "certificate_authority_names" {
  value = [for ca in data.fleetdm_certificate_authorities.all.certificate_authorities : ca.name]
}

# Look up a single certificate authority's id by name.
output "scep_wifi_id" {
  value = one([
    for ca in data.fleetdm_certificate_authorities.all.certificate_authorities :
    ca.id if ca.name == "SCEP_WIFI"
  ])
}
