# Look up a host by ID
data "fleetdm_host" "by_id" {
  id = 42
}

# Look up a host by identifier (hostname, UUID, or serial number)
data "fleetdm_host" "by_serial" {
  identifier = "C02ABC123456"
}

# Output host information
output "host_details" {
  value = {
    hostname        = data.fleetdm_host.by_id.hostname
    display_name    = data.fleetdm_host.by_id.display_name
    platform        = data.fleetdm_host.by_id.platform
    os_version      = data.fleetdm_host.by_id.os_version
    hardware_serial = data.fleetdm_host.by_id.hardware_serial
    status          = data.fleetdm_host.by_id.status
    team_name       = data.fleetdm_host.by_id.team_name
    primary_ip      = data.fleetdm_host.by_id.primary_ip
    disk_available  = data.fleetdm_host.by_id.gigs_disk_space_available
  }
}
