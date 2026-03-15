# Import a global policy using its ID
terraform import fleetdm_policy.disk_encryption 123

# For team policies, the import format is the policy ID
# You'll need to set team_id in the resource after import
terraform import fleetdm_policy.team_policy 456
