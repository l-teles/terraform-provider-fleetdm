variable "scep_challenge" {
  type      = string
  sensitive = true
}

variable "est_password" {
  type      = string
  sensitive = true
}

variable "digicert_api_token" {
  type      = string
  sensitive = true
}

variable "ndes_password" {
  type      = string
  sensitive = true
}

variable "hydrant_client_secret" {
  type      = string
  sensitive = true
}

variable "smallstep_password" {
  type      = string
  sensitive = true
}

# Custom SCEP proxy, write-only credential (preferred, Terraform 1.11+).
#
# challenge_wo is never written to the plan or the state file. Terraform cannot
# see a write-only value change, so rotating it means editing challenge_wo *and*
# incrementing secrets_wo_version in the same change.
resource "fleetdm_certificate_authority" "scep_wifi" {
  secrets_wo_version = 1

  custom_scep_proxy = {
    name         = "SCEP_WIFI"
    url          = "https://scep.example.com/scep"
    challenge_wo = var.scep_challenge
  }
}

# The same CA using the in-state credential instead. Works on any Terraform
# version, but the secret is written to state — treat state as a secret store.
resource "fleetdm_certificate_authority" "scep_legacy" {
  custom_scep_proxy = {
    name      = "SCEP_LEGACY"
    url       = "https://scep.example.com/scep"
    challenge = var.scep_challenge
  }
}

# Custom EST proxy
resource "fleetdm_certificate_authority" "est_wifi" {
  secrets_wo_version = 1

  custom_est_proxy = {
    name        = "EST_WIFI"
    url         = "https://est.example.com/.well-known/est"
    username    = "fleet"
    password_wo = var.est_password
  }
}

# DigiCert ONE
resource "fleetdm_certificate_authority" "digicert" {
  secrets_wo_version = 1

  digicert = {
    name                    = "DIGICERT_WIFI"
    url                     = "https://one.digicert.com"
    api_token_wo            = var.digicert_api_token
    profile_id              = "00000000-0000-0000-0000-000000000000"
    certificate_common_name = "$FLEET_VAR_HOST_HARDWARE_SERIAL"
    certificate_seat_id     = "$FLEET_VAR_HOST_HARDWARE_SERIAL"

    certificate_user_principal_names = [
      "$FLEET_VAR_HOST_END_USER_EMAIL_IDP",
    ]
  }
}

# Microsoft NDES SCEP proxy. Fleet allows one NDES CA per server and fixes its
# name to "NDES", so this block takes no name.
resource "fleetdm_certificate_authority" "ndes" {
  secrets_wo_version = 1

  ndes_scep_proxy = {
    url         = "https://ndes.example.com/certsrv/mscep/mscep.dll"
    admin_url   = "https://ndes.example.com/certsrv/mscep_admin/"
    username    = "fleet@example.com"
    password_wo = var.ndes_password
  }
}

# Hydrant
resource "fleetdm_certificate_authority" "hydrant" {
  secrets_wo_version = 1

  hydrant = {
    name             = "HYDRANT_WIFI"
    url              = "https://hydrant.example.com"
    client_id        = "example-client-id"
    client_secret_wo = var.hydrant_client_secret
  }
}

# Smallstep
resource "fleetdm_certificate_authority" "smallstep" {
  secrets_wo_version = 1

  smallstep = {
    name          = "SMALLSTEP_WIFI"
    url           = "https://example.scep.smallstep.com/scep/agents"
    challenge_url = "https://example.scep.smallstep.com/challenge"
    username      = "fleet"
    password_wo   = var.smallstep_password
  }
}
