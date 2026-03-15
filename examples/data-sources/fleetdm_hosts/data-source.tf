# List all online hosts
data "fleetdm_hosts" "online" {
  status = "online"
}

# List hosts in a specific team
data "fleetdm_hosts" "workstations" {
  team_id = 1
}

# Search for hosts by name
data "fleetdm_hosts" "search" {
  query = "workstation"
}

# List macOS hosts that are online
data "fleetdm_hosts" "macos_online" {
  status   = "online"
  platform = "darwin"
}

# Get hosts matching a policy
data "fleetdm_hosts" "failing_disk_encryption" {
  policy_id = 5
}

# Paginated list
data "fleetdm_hosts" "page_2" {
  per_page = 50
  page     = 2
}

# Output host count and details
output "online_host_count" {
  value = length(data.fleetdm_hosts.online.hosts)
}

output "online_hosts" {
  value = [
    for host in data.fleetdm_hosts.online.hosts : {
      id       = host.id
      hostname = host.hostname
      platform = host.platform
      status   = host.status
    }
  ]
}
