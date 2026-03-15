# List all software titles
data "fleetdm_software_titles" "all" {}

output "total_software_count" {
  value = data.fleetdm_software_titles.all.total_count
}

output "software_names" {
  value = [for s in data.fleetdm_software_titles.all.software_titles : s.name]
}

# Filter software by team
data "fleetdm_software_titles" "team_software" {
  team_id = fleetdm_team.workstations.id
}

# Search for specific software
data "fleetdm_software_titles" "browsers" {
  query = "chrome"
}

output "browser_software" {
  value = [for s in data.fleetdm_software_titles.browsers.software_titles : {
    name   = s.name
    source = s.source
    hosts  = s.hosts_count
  }]
}
