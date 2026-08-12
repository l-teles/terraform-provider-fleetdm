# List the certificate templates targeting hosts that are not assigned to a
# fleet. Fleet's list endpoint is scoped to a single fleet, so omitting fleet_id
# reads fleet 0 rather than every certificate on the server.
data "fleetdm_certificates" "unassigned" {}

# List one fleet's certificate templates.
data "fleetdm_certificates" "engineering" {
  fleet_id = fleetdm_fleet.engineering.id
}

output "engineering_certificate_names" {
  value = [for cert in data.fleetdm_certificates.engineering.certificates : cert.name]
}

# Look up a single certificate template's id by name.
output "wifi_engineering_id" {
  value = one([
    for cert in data.fleetdm_certificates.engineering.certificates :
    cert.id if cert.name == "WiFi_Engineering"
  ])
}

resource "fleetdm_fleet" "engineering" {
  name = "Engineering"
}
