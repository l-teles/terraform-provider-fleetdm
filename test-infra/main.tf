# =============================================================================
# FleetDM Terraform Provider - Full Infrastructure Test
# =============================================================================
#
# This file creates a complete set of FleetDM resources to test the provider.
# Resources are created in dependency order and can be destroyed cleanly.
#
# Usage:
#   terraform init
#   terraform plan
#   terraform apply
#   terraform destroy
#
# Command: cd /Users/luisteles/Git\ repos/drafts/fleetdm-tf-provider/terraform-provider-fleetdm/test-infra && rm -f terraform.tfstate terraform.tfstate.backup && TF_CLI_CONFIG_FILE=.terraformrc terraform apply -auto-approve -var='test_prefix=tf-test5' 2>&1
#
# =============================================================================

# -----------------------------------------------------------------------------
# Data Sources - Read existing data
# -----------------------------------------------------------------------------

# Get FleetDM server version
data "fleetdm_version" "current" {}

# List existing teams (to verify API connection)
data "fleetdm_teams" "all" {}

# -----------------------------------------------------------------------------
# Team - Container for all test resources
# -----------------------------------------------------------------------------

resource "fleetdm_team" "test" {
  name        = "${var.test_prefix}-team"
  description = "Terraform provider test team - safe to delete"

  # Team settings
  host_expiry_enabled = false

  enable_disk_encryption = false

}

# -----------------------------------------------------------------------------
# Label - Dynamic host grouping
# -----------------------------------------------------------------------------

resource "fleetdm_label" "macos" {
  name        = "${var.test_prefix}-macos-label"
  description = "Label for macOS hosts (Terraform test)"

  # Query-based dynamic label
  query    = "SELECT 1 FROM os_version WHERE platform = 'darwin';"
  platform = "darwin"
}

resource "fleetdm_label" "all_hosts" {
  name        = "${var.test_prefix}-all-hosts"
  description = "Label matching all hosts (Terraform test)"

  # Simple query that matches all hosts
  query    = "SELECT 1;"
  platform = "" # All platforms
}

# -----------------------------------------------------------------------------
# Query - Saved osquery
# -----------------------------------------------------------------------------

resource "fleetdm_query" "system_info" {
  name        = "${var.test_prefix}-system-info"
  description = "Get basic system information (Terraform test)"
  query       = "SELECT hostname, cpu_brand, physical_memory FROM system_info;"

  # Team assignment
  team_id = fleetdm_team.test.id

  # Query settings
  observer_can_run = true

  # Logging configuration
  logging = "snapshot"
}

resource "fleetdm_query" "os_version" {
  name        = "${var.test_prefix}-os-version"
  description = "Get OS version details (Terraform test)"
  query       = "SELECT name, version, major, minor, patch, platform FROM os_version;"

  team_id          = fleetdm_team.test.id
  observer_can_run = true
  logging          = "snapshot"
}

# -----------------------------------------------------------------------------
# Policy - Compliance checks
# -----------------------------------------------------------------------------

resource "fleetdm_policy" "disk_encryption" {
  name        = "${var.test_prefix}-disk-encryption"
  description = "Verify disk encryption is enabled (Terraform test)"

  # Policy query - returns results if policy FAILS
  query = <<-EOT
    SELECT 1 FROM disk_encryption 
    WHERE encrypted = 0 
    AND name LIKE '/dev/disk%';
  EOT

  # Team assignment
  team_id = fleetdm_team.test.id

  # Policy settings
  platform   = "darwin,linux"
  critical   = true
  resolution = "Enable FileVault on macOS or LUKS on Linux to encrypt your disk."
}

resource "fleetdm_policy" "screensaver_lock" {
  name        = "${var.test_prefix}-screensaver-lock"
  description = "Verify screensaver requires password (Terraform test)"

  query = <<-EOT
    SELECT 1 FROM managed_policies 
    WHERE domain = 'com.apple.screensaver' 
    AND name = 'askForPassword' 
    AND value != '1';
  EOT

  team_id    = fleetdm_team.test.id
  platform   = "darwin"
  critical   = false
  resolution = "Enable 'Require password after sleep or screen saver begins' in System Preferences > Security & Privacy."
}

# -----------------------------------------------------------------------------
# Script - Automation scripts
# -----------------------------------------------------------------------------

resource "fleetdm_script" "hello_world" {
  name    = "${var.test_prefix}-hello-world.sh"
  team_id = fleetdm_team.test.id

  # Script content (plain text, will be base64 encoded by provider)
  content = <<-EOT
    #!/bin/bash
    # Terraform Provider Test Script
    # This script is safe to run - it just prints info

    echo "=== FleetDM Terraform Provider Test ==="
    echo "Hostname: $(hostname)"
    echo "Date: $(date)"
    echo "User: $(whoami)"
    echo "OS: $(uname -s) $(uname -r)"
    echo "=== Test Complete ==="
  EOT
}

resource "fleetdm_script" "system_check" {
  name    = "${var.test_prefix}-system-check.sh"
  team_id = fleetdm_team.test.id

  content = <<-EOT
    #!/bin/bash
    # System health check script (Terraform test)

    echo "Checking system health..."

    # Check disk space
    echo "Disk Usage:"
    df -h / | tail -1

    # Check memory
    echo "Memory:"
    if command -v free &> /dev/null; then
        free -h
    else
        vm_stat | head -5
    fi

    echo "System check complete."
  EOT
}

# -----------------------------------------------------------------------------
# Software Package - Zoom
# -----------------------------------------------------------------------------

resource "fleetdm_software_package" "zoom" {
  team_id  = fleetdm_team.test.id
  filename = basename(var.package_path)

  # Package file path
  package_path = var.package_path

  # Installation scripts
  install_script = <<-EOT
    #!/bin/bash
    # Install Zoom
    sudo installer -pkg "$INSTALLER_PATH" -target /
  EOT

  uninstall_script = <<-EOT
    #!/bin/bash
    # Uninstall Zoom
    if [ -d "/Applications/zoom.us.app" ]; then
        sudo rm -rf "/Applications/zoom.us.app"
    fi
  EOT

  # Software settings
  self_service = true

  # Note: automatic_install requires additional setup
  # automatic_install = false
}

# -----------------------------------------------------------------------------
# Enroll Secret - For enrolling hosts to the test team
# -----------------------------------------------------------------------------

resource "fleetdm_enroll_secret" "test_team" {
  team_id = fleetdm_team.test.id
  secrets = [
    {
      secret = "${var.test_prefix}-enroll-secret-2026"
    }
  ]
}
