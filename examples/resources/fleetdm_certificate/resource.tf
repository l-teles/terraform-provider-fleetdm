# Certificate templates issue client certificates to enrolled Android hosts.
# They require a custom SCEP proxy certificate authority — no other type is
# supported on Fleet 4.90.
resource "fleetdm_certificate_authority" "wifi_scep" {
  custom_scep_proxy = {
    name         = "SCEP_WIFI"
    url          = "https://scep.example.com/scep"
    challenge_wo = var.scep_challenge
  }

  secrets_wo_version = 1
}

# A certificate for hosts that are not assigned to a fleet.
resource "fleetdm_certificate" "wifi_default" {
  name                     = "WiFi_Default"
  certificate_authority_id = fleetdm_certificate_authority.wifi_scep.id
  subject_name             = "CN=$FLEET_VAR_HOST_HARDWARE_SERIAL,O=Example Corp"
}

# A fleet-scoped certificate that identifies the end user rather than the device.
# Fleet expands each $FLEET_VAR_* reference per host at delivery time.
resource "fleetdm_certificate" "wifi_engineering" {
  fleet_id                 = fleetdm_fleet.engineering.id
  name                     = "WiFi_Engineering"
  certificate_authority_id = fleetdm_certificate_authority.wifi_scep.id
  subject_name             = "CN=$FLEET_VAR_HOST_END_USER_IDP_USERNAME,OU=Engineering,O=Example Corp"

  # Subject alternative names are a comma-separated list of KEY=value entries.
  # Allowed keys are DNS, EMAIL, UPN, IP and URI.
  subject_alternative_name = "UPN=$FLEET_VAR_HOST_END_USER_IDP_USERNAME,DNS=$FLEET_VAR_HOST_UUID.example.com"
}

resource "fleetdm_fleet" "engineering" {
  name = "Engineering"
}

variable "scep_challenge" {
  type      = string
  sensitive = true
}
