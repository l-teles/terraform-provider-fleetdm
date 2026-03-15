# List all software versions
data "fleetdm_software_versions" "all" {}

output "total_versions_count" {
  value = data.fleetdm_software_versions.all.total_count
}

# Filter by vulnerable software only
data "fleetdm_software_versions" "vulnerable" {
  vulnerable = true
}

output "vulnerable_software" {
  value = [for v in data.fleetdm_software_versions.vulnerable.software_versions : {
    id              = v.id
    version         = v.version
    hosts_count     = v.hosts_count
    vulnerabilities = v.vulnerabilities
  }]
}

# Filter by team
data "fleetdm_software_versions" "team_versions" {
  team_id = fleetdm_team.workstations.id
}
