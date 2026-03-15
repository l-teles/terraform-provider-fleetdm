# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

> **Note**: Release notes are automatically generated via [Release Drafter](https://github.com/release-drafter/release-drafter)
> from PR labels. See `.github/release-drafter.yml` for the configuration.

## [Unreleased]

### Added

#### Provider

- Provider configuration with `server_address`, `api_key`, `verify_tls`, and `timeout` options
- Environment variable support: `FLEETDM_URL`, `FLEETDM_API_TOKEN`, `FLEETDM_VERIFY_TLS`
- Support for FleetDM Free and Premium editions

#### Resources (12)

- `fleetdm_team` - Manage teams with host expiry and disk encryption settings (Premium)
- `fleetdm_label` - Manage dynamic labels with query-based membership
- `fleetdm_query` - Manage saved queries with scheduling options
- `fleetdm_policy` - Manage compliance policies (global and team-scoped)
- `fleetdm_script` - Manage shell/PowerShell scripts
- `fleetdm_enroll_secret` - Manage enrollment secrets (global and team)
- `fleetdm_user` - Manage Fleet users and permissions
- `fleetdm_software_package` - Upload and manage software packages (Premium)
- `fleetdm_bootstrap_package` - Manage bootstrap packages for setup assistant (Premium)
- `fleetdm_configuration` - Manage global Fleet configuration
- `fleetdm_configuration_profile` - Manage MDM configuration profiles (Premium)
- `fleetdm_setup_experience` - Manage macOS setup experience settings (Premium)

#### Data Sources (26)

- `fleetdm_team` / `fleetdm_teams` - Read team information
- `fleetdm_label` / `fleetdm_labels` - Read label information
- `fleetdm_query` / `fleetdm_queries` - Read query information
- `fleetdm_policy` / `fleetdm_policies` - Read policy information (global and team)
- `fleetdm_host` / `fleetdm_hosts` - Read host information (by ID, identifier, or list)
- `fleetdm_script` / `fleetdm_scripts` - Read script information
- `fleetdm_software_title` / `fleetdm_software_titles` - Read software title information
- `fleetdm_software_version` / `fleetdm_software_versions` - Read software version information
- `fleetdm_user` / `fleetdm_users` - Read user information
- `fleetdm_activities` - Read activity log entries
- `fleetdm_configuration` - Get Fleet configuration
- `fleetdm_configuration_profiles` - Read MDM configuration profiles (Premium)
- `fleetdm_enroll_secrets` - Get enrollment secrets
- `fleetdm_version` - Get Fleet server version
- `fleetdm_mdm_summary` - Get MDM enrollment summary (Premium)
- `fleetdm_abm_tokens` - Read Apple Business Manager tokens (Premium)
- `fleetdm_vpp_tokens` - Read Apple Volume Purchase tokens (Premium)

### Features

- Full CRUD support for all resources
- Import support for all resources
- Acceptance test suite running against real Fleet instance in CI
- Example configurations for all resources and data sources
