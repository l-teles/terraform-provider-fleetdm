# Create a label that identifies all macOS hosts
resource "fleetdm_label" "macos_hosts" {
  name        = "macOS Hosts"
  description = "All hosts running macOS"
  query       = "SELECT 1 FROM os_version WHERE platform = 'darwin'"
  platform    = "darwin"
}

# Create a label for Windows servers
resource "fleetdm_label" "windows_servers" {
  name        = "Windows Servers"
  description = "Windows Server machines"
  query       = "SELECT 1 FROM os_version WHERE name LIKE 'Windows Server%'"
  platform    = "windows"
}

# Create a label for hosts with SSD storage
resource "fleetdm_label" "ssd_hosts" {
  name        = "SSD Storage"
  description = "Hosts with SSD storage"
  query       = "SELECT 1 FROM disk_info WHERE type = 'ssd'"
}

# Create a label for hosts with low disk space
resource "fleetdm_label" "low_disk_space" {
  name        = "Low Disk Space"
  description = "Hosts with less than 10GB free disk space"
  query       = "SELECT 1 FROM disk_info WHERE free_space < 10737418240"
}

# A host vitals label: membership follows a host attribute instead of a query.
# Here, the end user's IdP group.
resource "fleetdm_label" "engineering" {
  name        = "Engineering"
  description = "Hosts whose end user is in the Engineering IdP group"

  criteria = {
    vital = "end_user_idp_group"
    value = "Engineering"
  }
}

# Host vitals labels can also match a custom host vital's per-host value.
resource "fleetdm_custom_host_vital" "asset_tag" {
  name = "Asset tag"
}

resource "fleetdm_label" "leased_hardware" {
  name        = "Leased hardware"
  description = "Hosts whose asset tag marks them as leased"

  criteria = {
    vital                = "custom_host_vital"
    operator             = "LIKE"
    value                = "LEASE-%"
    custom_host_vital_id = fleetdm_custom_host_vital.asset_tag.id
  }
}

# A manual label: neither query nor criteria, with membership assigned
# host-by-host outside Terraform.
resource "fleetdm_label" "quarantine" {
  name        = "Quarantine"
  description = "Hosts pulled aside for investigation"
}
