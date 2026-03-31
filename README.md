# Terraform Provider for FleetDM

[![Tests](https://github.com/l-teles/terraform-provider-fleetdm/actions/workflows/test.yml/badge.svg)](https://github.com/l-teles/terraform-provider-fleetdm/actions/workflows/test.yml)
[![Release](https://github.com/l-teles/terraform-provider-fleetdm/actions/workflows/release.yml/badge.svg)](https://github.com/l-teles/terraform-provider-fleetdm/actions/workflows/release.yml)

> [!WARNING]
> **PREVIEW / EXPERIMENTAL — USE AT YOUR OWN RISK**
>
> This provider is in early preview and has **not been extensively tested** in production environments. It was built primarily through AI-assisted development ("vibecoding") using Claude Opus 4.5 and Claude 4.6, and still requires careful human review before use in any critical infrastructure.
>
> **This project has no affiliation with FleetDM, Inc. and is not officially supported by them.**
> Use of the name here refers only to API compatibility.
>
> Please review all code carefully before applying it to any environment. Contributions, bug reports, and reviews are very welcome.

This Terraform provider allows you to manage [FleetDM](https://fleetdm.com) resources using infrastructure-as-code.

## Features

> **Requires Fleet 4.82.0+.** All resources and data sources in this provider version — including the deprecated `fleetdm_team` and `fleetdm_query` aliases — route through the new Fleet API endpoints (`/fleets`, `/reports`). These endpoints are only available on Fleet 4.82.0+.

### Resources (14)

| Resource                        | Description                                                |
| ------------------------------- | ---------------------------------------------------------- |
| `fleetdm_fleet`                 | Manage fleets with host expiry and disk encryption settings |
| `fleetdm_label`                 | Manage dynamic labels for host grouping                    |
| `fleetdm_report`                | Manage osquery reports with scheduling                     |
| `fleetdm_policy`                | Manage compliance policies (global and fleet-scoped)       |
| `fleetdm_script`                | Manage shell/PowerShell scripts                            |
| `fleetdm_enroll_secret`         | Manage enrollment secrets (global and fleet)               |
| `fleetdm_user`                  | Manage Fleet users and permissions                         |
| `fleetdm_software_package`      | Upload and manage software packages (Premium)              |
| `fleetdm_bootstrap_package`     | Manage bootstrap packages for setup assistant (Premium)    |
| `fleetdm_configuration`         | Manage global Fleet configuration                          |
| `fleetdm_configuration_profile` | Manage MDM configuration profiles (Premium)                |
| `fleetdm_setup_experience`      | Manage macOS setup experience settings (Premium)           |
| `fleetdm_team` *(deprecated)*   | Deprecated alias for `fleetdm_fleet`                       |
| `fleetdm_query` *(deprecated)*  | Deprecated alias for `fleetdm_report`                      |

### Data Sources (30)

| Data Source                                              | Description                               |
| -------------------------------------------------------- | ----------------------------------------- |
| `fleetdm_fleet` / `fleetdm_fleets`                       | Read fleet information                    |
| `fleetdm_label` / `fleetdm_labels`                       | Read label information                    |
| `fleetdm_report` / `fleetdm_reports`                     | Read report information                   |
| `fleetdm_policy` / `fleetdm_policies`                    | Read policy information (global and fleet) |
| `fleetdm_host` / `fleetdm_hosts`                         | Read host information (by ID, identifier, or list) |
| `fleetdm_script` / `fleetdm_scripts`                     | Read script information                   |
| `fleetdm_software_title` / `fleetdm_software_titles`     | Read software title information           |
| `fleetdm_software_version` / `fleetdm_software_versions` | Read software version information         |
| `fleetdm_user` / `fleetdm_users`                         | Read user information                     |
| `fleetdm_activities`                                     | Read activity log entries                 |
| `fleetdm_configuration`                                  | Get Fleet configuration                   |
| `fleetdm_configuration_profiles`                         | Read MDM configuration profiles (Premium) |
| `fleetdm_enroll_secrets`                                 | Get enrollment secrets                    |
| `fleetdm_version`                                        | Get Fleet server version                  |
| `fleetdm_mdm_summary`                                    | Get MDM enrollment summary (Premium)      |
| `fleetdm_abm_tokens`                                     | Read Apple Business Manager tokens        |
| `fleetdm_vpp_tokens`                                     | Read Apple Volume Purchase tokens         |
| `fleetdm_team` / `fleetdm_teams` *(deprecated)*          | Deprecated aliases for `fleetdm_fleet` / `fleetdm_fleets` |
| `fleetdm_query` / `fleetdm_queries` *(deprecated)*       | Deprecated aliases for `fleetdm_report` / `fleetdm_reports` |

## Requirements

- [Terraform](https://www.terraform.io/downloads.html) >= 1.5
- [Go](https://golang.org/doc/install) >= 1.24 (for building)
- FleetDM server >= 4.82.0 (required for all resources — the provider now exclusively uses the new `/fleets` and `/reports` API endpoints)

## Installation

### From Terraform Registry

```hcl
terraform {
  required_providers {
    fleetdm = {
      source  = "l-teles/fleetdm"
      version = "~> 0.1"
    }
  }
}
```

### Building from Source

```bash
git clone https://github.com/l-teles/terraform-provider-fleetdm.git
cd terraform-provider-fleetdm
make install
```

### Using the Provider

```hcl
provider "fleetdm" {
  server_address = "https://fleet.example.com"
  api_key        = var.fleetdm_api_key
}
```

## Configuration

### Provider Arguments

| Argument         | Description                   | Required | Default |
| ---------------- | ----------------------------- | -------- | ------- |
| `server_address` | FleetDM server URL            | Yes\*    | -       |
| `api_key`        | API key for authentication    | Yes\*    | -       |
| `verify_tls`     | Verify TLS certificates       | No\*     | `true`  |
| `timeout`        | API request timeout (seconds) | No       | `30`    |

\*Can also be set via environment variables (see below).

### Environment Variables

```bash
export FLEETDM_URL="https://fleet.example.com"
export FLEETDM_API_TOKEN="your-api-key"
export FLEETDM_VERIFY_TLS="false"  # Optional: disable TLS verification
```

## Quick Start

### Create a Fleet

```hcl
resource "fleetdm_fleet" "workstations" {
  name        = "Workstations"
  description = "All workstation devices"

  host_expiry_enabled = true
  host_expiry_window  = 30
}
```

### Create a Label

```hcl
resource "fleetdm_label" "macos_hosts" {
  name        = "macOS Hosts"
  description = "All hosts running macOS"
  query       = "SELECT 1 FROM os_version WHERE platform = 'darwin'"
  platform    = "darwin"
}
```

### Create a Report

```hcl
resource "fleetdm_report" "os_version" {
  name        = "Get OS Version"
  description = "Returns OS version information"
  query       = "SELECT * FROM os_version"
  interval    = 3600  # Run every hour
  logging     = "snapshot"
}
```

### Create a Policy

```hcl
resource "fleetdm_policy" "disk_encryption" {
  name        = "Disk Encryption Enabled"
  description = "Verifies disk encryption is enabled"
  query       = "SELECT 1 FROM disk_encryption WHERE encrypted = 1"
  critical    = true
  resolution  = "Enable FileVault on macOS or BitLocker on Windows"
  platform    = "darwin,windows"
}
```

### Upload a Software Package (Premium)

```hcl
resource "fleetdm_software_package" "zoom" {
  team_id      = fleetdm_fleet.workstations.id
  filename     = "zoom-installer.pkg"
  package_path = "./packages/zoom-installer.pkg"

  install_script   = "installer -pkg /tmp/zoom-installer.pkg -target /"
  uninstall_script = "rm -rf /Applications/zoom.us.app"

  self_service = true
}
```

### Create a Script

```hcl
resource "fleetdm_script" "system_update" {
  team_id = fleetdm_fleet.workstations.id
  name    = "update-system.sh"
  content = file("${path.module}/scripts/update-system.sh")
}
```

## Development

### Prerequisites

- Go 1.24+
- Make

### Building

```bash
make build
```

### Testing

```bash
# Run unit tests
make test

# Run acceptance tests (requires FleetDM server)
export FLEETDM_URL="https://fleet.example.com"
export FLEETDM_API_TOKEN="your-api-key"
make testacc
```

### Installing Locally

```bash
make install
```

## Project Structure

```
terraform-provider-fleetdm/
├── internal/
│   ├── fleetdm/          # FleetDM API client
│   │   ├── client.go     # HTTP client implementation
│   │   ├── teams.go      # Fleets API (teams.go kept for compatibility)
│   │   ├── labels.go     # Labels API
│   │   ├── queries.go    # Reports API (queries.go kept for compatibility)
│   │   ├── policies.go   # Policies API
│   │   └── *_test.go     # Unit tests
│   └── provider/         # Terraform provider
│       ├── provider.go   # Provider configuration
│       ├── *_resource.go # Resource implementations
│       └── *_data_source.go # Data source implementations
├── examples/             # Example configurations
├── docs/                 # Generated documentation
├── main.go               # Provider entry point
├── go.mod                # Go module definition
└── GNUmakefile           # Build automation
```

## API Coverage

| Category      | API Operations              | Status         |
| ------------- | --------------------------- | -------------- |
| Fleets        | CRUD, Enroll Secrets        | ✅ Implemented |
| Labels        | CRUD                        | ✅ Implemented |
| Reports       | CRUD, Scheduling            | ✅ Implemented |
| Policies      | CRUD (Global & Fleet)       | ✅ Implemented |
| Scripts       | CRUD                        | ✅ Implemented |
| Hosts         | Read, Search, Filter        | ✅ Implemented |
| Users         | CRUD                        | ✅ Implemented |
| Software      | Read, Upload Packages       | ✅ Implemented |
| MDM           | Profiles, Bootstrap Package | ✅ Implemented |
| Configuration | Global Settings             | ✅ Implemented |
| Activities    | Read, Filter                | ✅ Implemented |
| Tokens        | ABM, VPP (Read-only)        | ✅ Implemented |

## Disclaimer

This project is an independent, community-created Terraform provider. It is **not** affiliated with, endorsed by, or supported by FleetDM, Inc. in any way. 

This provider was developed with significant AI assistance (Claude Opus 4.5 and Claude 4.6). While the code has been reviewed, it has **not been extensively tested** in production environments. Use it at your own risk and always review the code before applying changes to any infrastructure.

## Contributing

Contributions are welcome! Please read our contributing guidelines and submit pull requests.

## License

This project is licensed under the MPL-2.0 License - see the [LICENSE](LICENSE) file for details.

## Support

- [FleetDM Documentation](https://fleetdm.com/docs)
- [FleetDM API Reference](https://fleetdm.com/docs/rest-api/rest-api)
- [Issue Tracker](https://github.com/l-teles/terraform-provider-fleetdm/issues)
