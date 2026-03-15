# =============================================================================
# Provider Configuration
# =============================================================================

terraform {
  required_version = ">= 1.0"

  # Note: fleetdm provider is loaded via dev_overrides in .terraformrc
  # required_providers is not needed for dev testing
}

# Configure the FleetDM Provider
provider "fleetdm" {
  server_address = var.fleetdm_url
  api_key        = var.fleetdm_token
  verify_tls     = var.verify_tls
  timeout        = 120 # Higher timeout for large package uploads
}
