# FleetDM Terraform Provider - Infrastructure Test

This folder contains a complete test setup for the FleetDM Terraform provider.
It tests real resources against a live FleetDM instance.

## ⚠️ WARNING

This will create REAL resources in your FleetDM instance!
Make sure you're testing against a dev/test environment.

## Prerequisites

1. A running FleetDM instance with Premium features
2. An API token with admin privileges
3. The Zoom package file (`zoomusInstallerFull.pkg`)
4. Terraform installed

## Setup

1. Copy the example variables file:

   ```bash
   cp terraform.tfvars.example terraform.tfvars
   ```

2. Edit `terraform.tfvars` with your FleetDM credentials:

   ```hcl
   fleetdm_url   = "https://your-fleet-instance.example.com"
   fleetdm_token = "your-api-token"
   ```

3. Make sure the package file is in place:
   ```bash
   # The package should be at:
   # ../zoomusInstallerFull.pkg
   ```

## Usage

### Initialize

```bash
terraform init
```

### Plan (preview changes)

```bash
terraform plan
```

### Apply (create resources)

```bash
terraform apply
```

### Destroy (clean up)

```bash
terraform destroy
```

## What Gets Created

| Resource Type    | Name                | Description                   |
| ---------------- | ------------------- | ----------------------------- |
| Team             | tf-test-team        | Test team for all resources   |
| Label            | tf-test-macos-label | Dynamic label for macOS hosts |
| Query            | tf-test-query       | Sample osquery                |
| Policy           | tf-test-policy      | Security policy               |
| Script           | tf-test-script      | Sample shell script           |
| Software Package | Zoom App            | Zoom package                  |

## Cleanup

After testing, run:

```bash
terraform destroy -auto-approve
```

Then you can safely delete this entire folder:

```bash
cd .. && rm -rf test-infra/
```
