# Write-only value (recommended, Terraform >= 1.11): the secret is never
# written to the plan or the state file. Increment value_wo_version to rotate.
resource "fleetdm_custom_variable" "cert_password" {
  name             = "CERT_PASSWORD"
  value_wo         = var.cert_password
  value_wo_version = 1
}

# Value stored in Terraform state. Use this only when write-only attributes are
# unavailable — anyone who can read the state file can read the value.
resource "fleetdm_custom_variable" "vendor_api_password" {
  name  = "VENDOR_API_PASSWORD"
  value = var.vendor_api_password
}

# Reference a custom variable from a script or configuration profile with the
# FLEET_SECRET_ prefix, which Fleet adds itself.
resource "fleetdm_script" "enroll_vendor_agent" {
  name = "enroll-vendor-agent.sh"

  content = <<-EOT
    #!/bin/sh
    /usr/local/bin/vendor-agent enroll --password "$FLEET_SECRET_VENDOR_API_PASSWORD"
  EOT

  depends_on = [fleetdm_custom_variable.vendor_api_password]
}
