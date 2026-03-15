# Get a specific script by ID
data "fleetdm_script" "security_check" {
  id = 1
}

output "script_name" {
  value = data.fleetdm_script.security_check.name
}

output "script_created" {
  value = data.fleetdm_script.security_check.created_at
}
