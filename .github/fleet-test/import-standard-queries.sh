#!/usr/bin/env bash
# import-standard-queries.sh – Install fleetctl and import Fleet's standard
# query library into the running test instance.
#
# Usage:
#   FLEETDM_URL=http://localhost:8080 \
#   FLEETDM_API_TOKEN=<token> \
#   .github/fleet-test/import-standard-queries.sh
#
# Environment variables:
#   FLEETDM_URL         Fleet server address (default: http://localhost:8080)
#   FLEETDM_API_TOKEN   API token obtained from setup-fleet.sh

set -euo pipefail

FLEET_URL="${FLEETDM_URL:-http://localhost:8080}"
API_TOKEN="${FLEETDM_API_TOKEN}"

STANDARD_QUERY_LIBRARY_URL="https://raw.githubusercontent.com/fleetdm/fleet/main/docs/01-Using-Fleet/standard-query-library/standard-query-library.yml"

# ---------------------------------------------------------------------------
# 1. Install fleetctl
# ---------------------------------------------------------------------------
echo "Installing fleetctl..." >&2
curl -sSL https://fleetdm.com/resources/install-fleetctl.sh | bash
# The install script places the binary in ~/.fleetctl/ which is not in PATH by default.
export PATH="$HOME/.fleetctl:$PATH"
echo "fleetctl installed." >&2

# ---------------------------------------------------------------------------
# 2. Configure fleetctl with server address and API token
# ---------------------------------------------------------------------------
echo "Configuring fleetctl for ${FLEET_URL}..." >&2
fleetctl config set --address "$FLEET_URL"
fleetctl config set --token "$API_TOKEN"

# ---------------------------------------------------------------------------
# 3. Download the standard query library YAML
# ---------------------------------------------------------------------------
echo "Downloading standard query library..." >&2
curl -sL "$STANDARD_QUERY_LIBRARY_URL" -o /tmp/standard-query-library.yml

# ---------------------------------------------------------------------------
# 4. Apply the standard query library
# ---------------------------------------------------------------------------
echo "Applying standard query library..." >&2
fleetctl apply -f /tmp/standard-query-library.yml

echo "Standard query library imported successfully." >&2
