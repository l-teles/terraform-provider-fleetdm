# Example: FleetDM ABM Tokens Data Source

# Retrieve all ABM (Apple Business Manager) tokens
data "fleetdm_abm_tokens" "all" {}

# Output the tokens
output "abm_tokens" {
  value = data.fleetdm_abm_tokens.all.tokens
}

# Output tokens that need renewal soon
output "tokens_needing_renewal" {
  value = [
    for token in data.fleetdm_abm_tokens.all.tokens : {
      id            = token.id
      org_name      = token.organization_name
      renew_date    = token.renew_date
      terms_expired = token.terms_expired
    }
    if token.terms_expired
  ]
}

# Use ABM token info for team configuration
resource "fleetdm_team" "managed_macs" {
  name        = "Managed macOS"
  description = "Team for ABM-enrolled macOS devices"
}
