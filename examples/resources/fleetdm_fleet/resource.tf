# Create a basic fleet
resource "fleetdm_fleet" "workstations" {
  name        = "Workstations"
  description = "All workstation devices"
}

# Create a fleet with host expiry settings
resource "fleetdm_fleet" "servers" {
  name        = "Servers"
  description = "Production servers"

  host_expiry_enabled = true
  host_expiry_window  = 30 # Days
}

# Create a fleet with per-platform disk encryption (Fleet 4.92 or later).
#
# Each platform is configured on its own, and settings you leave out keep
# whatever value they already have in Fleet. The deprecated top-level
# enable_disk_encryption sets all four at once and cannot be combined with
# these attributes.
resource "fleetdm_fleet" "secure_workstations" {
  name        = "Secure Workstations"
  description = "Workstations with enhanced security"

  host_expiry_enabled = true
  host_expiry_window  = 14

  mdm = {
    macos_settings = {
      enable_disk_encryption            = true
      enable_escrow_disk_encryption_key = true
    }
    windows_settings = {
      enable_disk_encryption = true
      # Fleet only accepts a BitLocker PIN requirement while Windows disk
      # encryption is enabled.
      require_bitlocker_pin = true
    }
    linux_settings = {
      enable_escrow_disk_encryption_key = true
    }
  }
}

# Create a fleet with webhook, MDM, integration and feature settings.
#
# These four blocks are opt-in: a block you do not declare is neither sent to
# Fleet nor read back into state, so this resource can share a fleet with
# settings managed in the Fleet UI or through GitOps. Within the `mdm` and
# `features` blocks the same holds per attribute — anything you leave out keeps
# whatever value it already has in Fleet.
#
# The exception is a `webhook_settings` sub-block or `integrations.google_calendar`:
# Fleet replaces those objects wholesale, so once you declare one, every one of
# its attributes is written and the ones you omit are written as zero values.
# Declare those sub-blocks in full.
resource "fleetdm_fleet" "managed_laptops" {
  name        = "Managed Laptops"
  description = "Laptops with OS update enforcement and webhook automations"

  # Prefer https: webhook payloads carry host identifiers, and webhook URLs
  # often embed a secret token in the path.
  webhook_settings = {
    failing_policies_webhook = {
      enable_failing_policies_webhook = true
      destination_url                 = "https://automation.example.com/fleet/failing-policies"
      policy_ids                      = [] # omitted attributes are written as zero values
      host_batch_size                 = 100
    }
    host_status_webhook = {
      enable_host_status_webhook = true
      destination_url            = "https://automation.example.com/fleet/host-status"
      host_percentage            = 10
      days_count                 = 3
    }
  }

  mdm = {
    name_template = "$FLEET_VAR_HOST_HARDWARE_SERIAL"

    # minimum_version must be a version Apple still publishes, given exactly:
    # Fleet checks it against Apple's Software Lookup Service.
    macos_updates = {
      minimum_version = "26.6.1"
      deadline        = "2026-12-01"
    }

    # deadline_days (0-30) and grace_period_days (0-7) must be set together.
    windows_updates = {
      deadline_days     = 7
      grace_period_days = 2
    }
  }

  integrations = {
    # Enabling either of these requires the matching integration to already
    # exist in the global Fleet configuration.
    conditional_access_enabled = false
  }

  features = {
    historical_data = {
      vulnerabilities = true
    }
  }
}
