# Get a specific software title by ID
data "fleetdm_software_title" "chrome" {
  id = 1
}

output "software_name" {
  value = data.fleetdm_software_title.chrome.name
}

output "software_source" {
  value = data.fleetdm_software_title.chrome.source
}

output "versions_count" {
  value = data.fleetdm_software_title.chrome.versions_count
}

output "hosts_count" {
  value = data.fleetdm_software_title.chrome.hosts_count
}
