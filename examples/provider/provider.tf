# FleetDM Provider Configuration

# Configure the FleetDM provider using environment variables:
# - FLEETDM_URL: The FleetDM server address
# - FLEETDM_API_TOKEN: Your API key
# - FLEETDM_VERIFY_TLS: Set to "false" or "0" to skip TLS verification (optional)

provider "fleetdm" {}

# Alternatively, configure explicitly (not recommended for production):
# provider "fleetdm" {
#   server_address = "https://fleet.example.com"
#   api_key        = var.fleetdm_api_key
#   verify_tls     = true
#   timeout        = 30
# }
