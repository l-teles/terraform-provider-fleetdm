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
