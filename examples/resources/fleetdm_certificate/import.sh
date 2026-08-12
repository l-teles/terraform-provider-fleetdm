# Import a certificate template using "fleet_id:id".
# Both parts are required because Fleet never returns a template's fleet in any
# response, so the template id alone cannot recover it.
terraform import fleetdm_certificate.wifi_engineering 3:42

# Certificates targeting hosts that are not assigned to a fleet use fleet_id 0
terraform import fleetdm_certificate.wifi_default 0:7
