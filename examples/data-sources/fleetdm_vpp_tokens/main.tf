# Example: FleetDM VPP Tokens Data Source

# Retrieve all VPP (Volume Purchase Program) tokens
data "fleetdm_vpp_tokens" "all" {}

# Output the tokens
output "vpp_tokens" {
  value = data.fleetdm_vpp_tokens.all.tokens
}

# Output token details with associated teams
output "vpp_token_details" {
  value = [
    for token in data.fleetdm_vpp_tokens.all.tokens : {
      id         = token.id
      org_name   = token.organization_name
      location   = token.location
      renew_date = token.renew_date
      team_count = length(token.teams)
    }
  ]
}
