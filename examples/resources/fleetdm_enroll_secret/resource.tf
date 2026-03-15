# Manage global enrollment secrets
resource "fleetdm_enroll_secret" "global" {
  secrets = [
    { secret = "my-global-enroll-secret-1" },
    { secret = "my-global-enroll-secret-2" },
  ]
}

# Manage team-specific enrollment secrets
resource "fleetdm_enroll_secret" "workstations" {
  team_id = fleetdm_team.workstations.id

  secrets = [
    { secret = "workstations-secret-prod" },
    { secret = "workstations-secret-staging" },
  ]
}

# Example: Generate random secrets
resource "random_password" "enroll_secret" {
  length  = 32
  special = false
}

resource "fleetdm_enroll_secret" "generated" {
  secrets = [
    { secret = random_password.enroll_secret.result },
  ]
}
