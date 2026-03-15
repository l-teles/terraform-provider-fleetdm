# Get a specific software version by ID
data "fleetdm_software_version" "chrome_latest" {
  id = 1
}

output "version" {
  value = data.fleetdm_software_version.chrome_latest.version
}

output "hosts_count" {
  value = data.fleetdm_software_version.chrome_latest.hosts_count
}

output "vulnerabilities" {
  value = data.fleetdm_software_version.chrome_latest.vulnerabilities
}
