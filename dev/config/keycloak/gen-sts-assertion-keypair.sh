#!/usr/bin/env bash
# Generate the throwaway RSA keypair used by TestOAuthSTSAssertion (D4) for RFC 7523
# private-key-JWT client authentication during the STS token exchange.
#
# The private key is NEVER committed: it is generated on demand into a gitignored
# folder (test/e2e/testdata/generated/). Keycloak's mcp-server-jwt client is rendered
# to trust the matching certificate at `make keycloak-install` time, so the key and
# the realm cert always stay in sync. These are dev/test credentials only.
set -euo pipefail

# Resolve the repo root from this script's location (dev/config/keycloak/) so the
# script works regardless of the caller's working directory.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
OUT_DIR="${REPO_ROOT}/test/e2e/testdata/generated"
REL_DIR="test/e2e/testdata/generated"
CERT="${OUT_DIR}/sts-assertion.crt"
KEY="${OUT_DIR}/sts-assertion.key"

FORCE=0
if [[ "${1:-}" == "--force" ]]; then
  FORCE=1
fi

if [[ "${FORCE}" -eq 0 && -f "${CERT}" && -f "${KEY}" ]]; then
  echo "exists: STS assertion keypair already present at ${REL_DIR}/ (use --force to regenerate)"
  exit 0
fi

mkdir -p "${OUT_DIR}"
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout "${KEY}" \
  -out "${CERT}" \
  -days 36500 \
  -subj "/CN=mcp-server-jwt" >/dev/null 2>&1
chmod 600 "${KEY}"

echo "generated: STS assertion keypair at ${REL_DIR}/ (key is gitignored, never committed)"
