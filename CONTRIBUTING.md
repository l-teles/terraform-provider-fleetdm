# Contributing to terraform-provider-fleetdm

Thank you for your interest in contributing to the FleetDM Terraform Provider!

## Development Requirements

- [Go](https://golang.org/doc/install) >= 1.24
- [Terraform](https://www.terraform.io/downloads.html) >= 1.5
- A FleetDM instance for testing (can use Docker)

## Setting Up Your Development Environment

1. Clone the repository:

   ```bash
   git clone https://github.com/fleetdm/terraform-provider-fleetdm.git
   cd terraform-provider-fleetdm
   ```

2. Install dependencies:

   ```bash
   go mod download
   ```

3. Build the provider:
   ```bash
   go build -o terraform-provider-fleetdm
   ```

## Running Tests

### Unit Tests

```bash
go test -v ./internal/...
```

### Integration Tests

Integration tests require a running FleetDM instance:

```bash
export FLEETDM_URL="https://your-fleet-server.com"
export FLEETDM_API_TOKEN="your-api-token"
go run ./cmd/integration_test
```

### Acceptance Tests

Acceptance tests run against a real FleetDM instance. In CI, a local Fleet instance is
started automatically via Docker Compose (see `.github/fleet-test/`). To run them locally:

```bash
export TF_ACC=1
export FLEETDM_URL="https://your-fleet-server.com"
export FLEETDM_API_TOKEN="your-api-token"
go test -v ./internal/provider/...
```

#### MDM-dependent tests

Some Fleet features (disk encryption, MDM profiles, bootstrap packages, etc.) require MDM
to be fully configured. The standard `acceptance` CI job runs against a minimal Fleet dev
instance that **does not have MDM configured**, so tests that require MDM are excluded from
the default test configs (e.g. `enable_disk_encryption` is not exercised in
`TestAccTeamResource_withSettings`).

If you have access to a Fleet instance with MDM configured, enable the optional
`acceptance-external` CI job by setting the `FLEETDM_URL` repository variable and the
`FLEETDM_API_TOKEN` repository secret. The job is defined in `.github/workflows/test.yml`
under `acceptance-external` and is disabled by default (`if: false`). Change that
condition to use the secrets check commented out above it.

## Local Provider Installation

To test the provider locally with Terraform:

1. Build the provider:

   ```bash
   go build -o terraform-provider-fleetdm
   ```

2. Create a `.terraformrc` file in your home directory:

   ```hcl
   provider_installation {
     dev_overrides {
       "fleetdm/fleetdm" = "/path/to/terraform-provider-fleetdm"
     }
     direct {}
   }
   ```

3. Use the provider in your Terraform configuration:
   ```hcl
   terraform {
     required_providers {
       fleetdm = {
         source = "fleetdm/fleetdm"
       }
     }
   }
   ```

## Code Structure

```
terraform-provider-fleetdm/
├── cmd/
│   └── integration_test/    # Integration tests
├── internal/
│   ├── fleetdm/             # API client
│   │   ├── client.go
│   │   ├── hosts.go
│   │   ├── teams.go
│   │   └── ...
│   └── provider/            # Terraform provider
│       ├── provider.go
│       ├── *_resource.go    # Resources
│       └── *_data_source.go # Data sources
├── examples/                # Example configurations
└── docs/                    # Documentation
```

## Adding a New Resource

1. Create the API client methods in `internal/fleetdm/`
2. Create the resource in `internal/provider/<name>_resource.go`
3. Register in `internal/provider/provider.go`
4. Add unit tests
5. Add example in `examples/resources/`
6. Add documentation in `docs/resources/`

## Adding a New Data Source

1. Create the API client methods in `internal/fleetdm/`
2. Create the data source in `internal/provider/<name>_data_source.go`
3. Register in `internal/provider/provider.go`
4. Add unit tests
5. Add example in `examples/data-sources/`
6. Add documentation in `docs/data-sources/`

## Commit Messages

Please use conventional commit messages:

- `feat:` for new features
- `fix:` for bug fixes
- `docs:` for documentation changes
- `test:` for test changes
- `refactor:` for refactoring

## Pull Request Process

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add/update tests
5. Run all tests
6. Submit a pull request

## Code of Conduct

Please be respectful and constructive in all interactions.

## License

By contributing, you agree that your contributions will be licensed under the MPL-2.0 License.
