# Example: FleetDM REST API Endpoints Data Source

# Retrieve Fleet's REST API endpoint catalog. This is the authoritative list of
# method/path pairs that a fleetdm_user's api_endpoints scope may reference.
data "fleetdm_rest_api_endpoints" "all" {}

# Output the full catalog
output "rest_api_endpoints" {
  value = data.fleetdm_rest_api_endpoints.all.api_endpoints
}

# Endpoints Fleet has deprecated, which are best avoided in new scopes
output "deprecated_endpoints" {
  value = [
    for endpoint in data.fleetdm_rest_api_endpoints.all.api_endpoints :
    "${endpoint.method} ${endpoint.path}"
    if endpoint.deprecated
  ]
}

# Build a read-only scope from every non-deprecated GET endpoint
locals {
  read_only_endpoints = [
    for endpoint in data.fleetdm_rest_api_endpoints.all.api_endpoints : {
      method = endpoint.method
      path   = endpoint.path
    }
    if endpoint.method == "GET" && !endpoint.deprecated
  ]
}

# An API-only user restricted to that scope. Calls to anything outside it are
# rejected by Fleet with a 403.
resource "fleetdm_user" "read_only_bot" {
  name        = "Read-only bot"
  email       = "read-only-bot@example.com"
  password    = var.read_only_bot_password
  global_role = "observer"
  api_only    = true

  api_endpoints = local.read_only_endpoints
}

variable "read_only_bot_password" {
  description = "Password for the read-only API bot user."
  type        = string
  sensitive   = true
}

# Fleet returns the API token once, at creation, and never again on read.
output "read_only_bot_token" {
  value     = fleetdm_user.read_only_bot.token
  sensitive = true
}
