# Get global enrollment secrets
data "fleetdm_enroll_secrets" "global" {}

output "global_secret_count" {
  value = length(data.fleetdm_enroll_secrets.global.secrets)
}

# Note: Secrets are sensitive and should be handled carefully
output "global_secrets" {
  value     = [for s in data.fleetdm_enroll_secrets.global.secrets : s.secret]
  sensitive = true
}

# Get team-specific enrollment secrets
data "fleetdm_enroll_secrets" "team" {
  team_id = fleetdm_team.workstations.id
}

output "team_secret_count" {
  value = length(data.fleetdm_enroll_secrets.team.secrets)
}

# Example: Use secrets for osquery deployment
locals {
  osquery_enroll_secret = length(data.fleetdm_enroll_secrets.global.secrets) > 0 ? data.fleetdm_enroll_secrets.global.secrets[0].secret : ""
}
