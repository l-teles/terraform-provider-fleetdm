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
#   FLEETCTL_INSTALL_DIR where the fleetctl binary goes (default: ~/.fleetctl)

set -euo pipefail

FLEET_URL="${FLEETDM_URL:-http://localhost:8080}"
API_TOKEN="${FLEETDM_API_TOKEN}"
FLEETCTL_VERSION="${FLEETCTL_VERSION:-latest}"

STANDARD_QUERY_LIBRARY_URL="https://raw.githubusercontent.com/fleetdm/fleet/main/docs/01-Using-Fleet/standard-query-library/standard-query-library.yml"

# ---------------------------------------------------------------------------
# 1. Install fleetctl
# ---------------------------------------------------------------------------
# Downloads the release tarball and verifies it against the release's
# checksums.txt, so no third-party install script is executed. Each download
# is retried because GitHub release downloads occasionally return 5xx.
FLEETCTL_INSTALL_DIR="${FLEETCTL_INSTALL_DIR:-$HOME/.fleetctl}"
GITHUB_RELEASES="https://github.com/fleetdm/fleet/releases"

fetch() {
  local url="$1" out="$2" attempt
  for attempt in 1 2 3; do
    if curl -sSfL --retry 0 "$url" -o "$out"; then
      return 0
    fi
    if [[ "$attempt" -eq 3 ]]; then
      echo "download failed after ${attempt} attempts: ${url}" >&2
      return 1
    fi
    echo "download attempt ${attempt} failed for ${url}; retrying..." >&2
    sleep $((attempt * 10))
  done
}

install_fleetctl() {
  local version="$1" arch os archive tmp expected actual sha256
  if [[ "$version" == "latest" ]]; then
    # The /latest URL redirects to the newest non-prerelease tag.
    version="$(curl -sSfI "${GITHUB_RELEASES}/latest" | grep -i '^location:' | grep -o 'fleet-v[0-9][^[:space:]]*' | sed 's/^fleet-v//')"
    [[ -n "$version" ]] || { echo "could not resolve the latest fleetctl version" >&2; return 1; }
  fi
  version="${version#v}"
  case "$(uname -m)" in
    arm64 | aarch64) arch="arm64" ;;
    *) arch="amd64" ;;
  esac
  case "$(uname -s)" in
    Linux*) os="linux_${arch}"; sha256="sha256sum" ;;
    Darwin*) os="macos"; sha256="shasum -a 256" ;;
    *) echo "unsupported operating system: $(uname -s)" >&2; return 1 ;;
  esac
  archive="fleetctl_v${version}_${os}"
  tmp="$(mktemp -d)"
  fetch "${GITHUB_RELEASES}/download/fleet-v${version}/${archive}.tar.gz" "${tmp}/${archive}.tar.gz" || return 1
  fetch "${GITHUB_RELEASES}/download/fleet-v${version}/checksums.txt" "${tmp}/checksums.txt" || return 1
  expected="$(grep " ${archive}.tar.gz\$" "${tmp}/checksums.txt" | cut -d' ' -f1)"
  actual="$($sha256 "${tmp}/${archive}.tar.gz" | cut -d' ' -f1)"
  if [[ -z "$expected" || "$expected" != "$actual" ]]; then
    echo "checksum mismatch for ${archive}.tar.gz (expected '${expected}', got '${actual}')" >&2
    return 1
  fi
  tar -xzf "${tmp}/${archive}.tar.gz" -C "$tmp" --strip-components=1 "${archive}/fleetctl"
  mkdir -p "$FLEETCTL_INSTALL_DIR"
  install -m 0755 "${tmp}/fleetctl" "${FLEETCTL_INSTALL_DIR}/fleetctl"
  rm -rf "$tmp"
  echo "fleetctl $("${FLEETCTL_INSTALL_DIR}/fleetctl" --version | head -1 | sed 's/^fleetctl - version //') installed in ${FLEETCTL_INSTALL_DIR}" >&2
}

echo "Installing fleetctl ${FLEETCTL_VERSION}..." >&2
install_fleetctl "$FLEETCTL_VERSION"
export PATH="${FLEETCTL_INSTALL_DIR}:$PATH"

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
