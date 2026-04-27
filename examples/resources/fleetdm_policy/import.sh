# Import a global policy by its ID.
terraform import fleetdm_policy.disk_encryption 123

# Import a team policy with the format "<team_id>:<policy_id>".
terraform import fleetdm_policy.team_policy 7:456
