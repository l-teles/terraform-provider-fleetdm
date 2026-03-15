# =============================================================================
# Variables for FleetDM Terraform Provider Test
# =============================================================================

variable "fleetdm_url" {
  description = "The URL of your FleetDM server"
  type        = string
}

variable "fleetdm_token" {
  description = "API token for FleetDM authentication"
  type        = string
  sensitive   = true
}

variable "verify_tls" {
  description = "Whether to verify TLS certificates"
  type        = bool
  default     = false
}

variable "test_prefix" {
  description = "Prefix for all test resources (makes cleanup easier)"
  type        = string
  default     = "tf-test"
}

variable "package_path" {
  description = "Path to the software package file"
  type        = string
  default     = "../zoomusInstallerFull.pkg"
}
