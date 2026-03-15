# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Terraform provider for [FleetDM](https://fleetdm.com) — manages Fleet resources via infrastructure-as-code. Built with the Terraform Plugin Framework (v1.17), Go 1.24, Protocol v6. Not affiliated with FleetDM, Inc.

## Common Commands

```bash
# Build
make build

# Run all unit tests (no Fleet server needed)
go test -v -cover -timeout=10m ./internal/...

# Run a single test
go test -v -run TestAccTeamResource_basic ./internal/provider/

# Run acceptance tests (requires live Fleet instance)
TF_ACC=1 go test -v -cover -timeout=25m ./internal/provider/...

# Acceptance tests need these env vars:
#   FLEETDM_URL=http://localhost:8080
#   FLEETDM_API_TOKEN=<api-token>

# Install provider locally for manual testing
make install

# Generate docs (requires terraform CLI)
make docs    # runs: go generate ./...

# Format / lint
make fmt     # gofmt -s -w .
make lint    # golangci-lint run ./...
```

## Architecture

### Two-layer design

1. **API Client** (`internal/fleetdm/`) — Pure Go HTTP client wrapping the Fleet REST API. Each domain has its own file (teams.go, queries.go, policies.go, etc.) with corresponding `_test.go` unit tests that use `httptest.NewServer` mocks.

2. **Provider** (`internal/provider/`) — Terraform Plugin Framework resources and data sources. Each resource/data source is a single file following the naming convention `{name}_resource.go` or `{name}_data_source.go`, with acceptance tests in `{name}_resource_test.go` / `{name}_data_source_test.go`.

### Resource implementation pattern

Every resource follows the same structure:
- Struct holding `*fleetdm.Client`
- Compile-time interface checks: `var _ resource.Resource = &XResource{}`
- `Metadata`, `Schema`, `Create`, `Read`, `Update`, `Delete` methods
- Most resources implement `ResourceWithImportState`
- The `Configure` method type-asserts `req.ProviderData` to `*fleetdm.Client`

### Provider configuration

`provider.go` reads `server_address` and `api_key` from config or env vars (`FLEETDM_URL`, `FLEETDM_API_TOKEN`). `verify_tls` can also be set via `FLEETDM_VERIFY_TLS` env var. `timeout` is config-only (no env var fallback).

### Test conventions

- **Every new feature, resource, data source, or behavioral change must include corresponding tests.** This is non-negotiable — no code ships without test coverage.
- Two test types exist side-by-side:
  - **Mock-server tests** use `httptest.NewServer` — run without a Fleet instance, validate request/response mapping and error handling
  - **Live acceptance tests** use `testAccPreCheck(t)` which skips if env vars are missing — run against a real Fleet instance in CI
- Test names follow `TestAcc{Resource}_{scenario}` for acceptance, standard Go naming for unit tests
- Acceptance tests use `testAccProtoV6ProviderFactories` and `providerConfig()` from `provider_test.go`
- Test resource names use random suffixes via `acctest.RandStringFromCharSet`
- Tests cover create, read, update, delete, and import state

### PR labels (required for release notes)

Every PR **must** have at least one label that maps to a release drafter category (`.github/release-drafter.yml`):
- `enhancement` or `feature` → Features (bumps minor version)
- `bug` or `fix` → Bug Fixes (bumps patch version)
- `documentation` → Documentation (bumps patch version)
- `chore` or `dependencies` → Maintenance (bumps patch version)
- `major` or `breaking-change` → triggers major version bump

Dependabot PRs get labels automatically. For all other PRs, add the appropriate label before merging.

### CI pipeline (`.github/workflows/test.yml`)

- **build**: compile + golangci-lint
- **generate**: verify `go generate` produces no diff
- **test**: unit tests (no `TF_ACC`)
- **acceptance**: spins up Fleet + MySQL + Redis + MinIO via Docker Compose (`.github/fleet-test/`), runs setup script to create admin user and get API token, then runs acceptance tests against Terraform 1.5/1.6/1.7. MDM features cannot be tested in dev mode.

### Documentation generation

`main.go` has `go:generate` directives that run `tfplugindocs`. Example configs in `examples/` and templates in `templates/` feed into generated `docs/`.

## Key Files

- Entry point: `main.go`
- Provider: `internal/provider/provider.go`
- API client: `internal/fleetdm/client.go` (base HTTP methods: Get, Post, Patch, Delete)
- CI Fleet setup: `.github/fleet-test/docker-compose.yml`, `.github/fleet-test/setup-fleet.sh`
- Linter config: `.golangci.yml` (suppresses `errcheck` in test files)
