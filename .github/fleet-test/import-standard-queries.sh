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
#   FLEETCTL_VERSION    fleetctl version to install, e.g. v4.92.1 (default: latest)

set -euo pipefail

FLEET_URL="${FLEETDM_URL:-http://localhost:8080}"
API_TOKEN="${FLEETDM_API_TOKEN}"
FLEETCTL_VERSION="${FLEETCTL_VERSION:-latest}"

STANDARD_QUERY_LIBRARY_URL="https://raw.githubusercontent.com/fleetdm/fleet/main/docs/01-Using-Fleet/standard-query-library/standard-query-library.yml"

# ---------------------------------------------------------------------------
# 1. Install fleetctl
# ---------------------------------------------------------------------------
# Retried because GitHub release downloads occasionally return 5xx.
echo "Installing fleetctl ${FLEETCTL_VERSION}..." >&2
for attempt in 1 2 3; do
  if curl -sSfL https://fleetdm.com/resources/install-fleetctl.sh | FLEETCTL_VERSION="$FLEETCTL_VERSION" bash; then
    break
  fi
  if [[ "$attempt" -eq 3 ]]; then
    echo "fleetctl install failed after ${attempt} attempts." >&2
    exit 1
  fi
  echo "fleetctl install attempt ${attempt} failed; retrying..." >&2
  sleep $((attempt * 10))
done
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

# --- TEMPORARY WORKAROUND (remove when Fleet issue #43025 is fixed) ----------
# Fleet v4.83.0 inserts an empty string for the `type` column when policies
# omit it, which MySQL strict mode rejects (Error 1265).  Inject
# `type: dynamic` into every policy spec so the INSERT succeeds.
echo "Patching standard query library (injecting default policy type)..." >&2
python3 -c "
import yaml, sys

docs = list(yaml.safe_load_all(open('/tmp/standard-query-library.yml')))
patched = 0
for doc in docs:
    if doc and doc.get('kind') == 'policy':
        spec = doc.get('spec', {})
        if 'type' not in spec:
            spec['type'] = 'dynamic'
            patched += 1
with open('/tmp/standard-query-library.yml', 'w') as f:
    yaml.dump_all(docs, f, default_flow_style=False, sort_keys=False)
print(f'Patched {patched} policies with default type.', file=sys.stderr)
"
# --- END TEMPORARY WORKAROUND ----------------------------------------------

echo "Applying standard query library..." >&2
fleetctl apply -f /tmp/standard-query-library.yml

echo "Standard query library imported successfully." >&2
