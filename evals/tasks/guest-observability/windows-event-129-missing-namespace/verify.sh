#!/usr/bin/env bash

set -euo pipefail

LOKI_PORT="${GUEST_OBSERVABILITY_LOKI_PORT:-3101}"
REQUESTS=""

for _ in 1 2 3 4 5; do
  if REQUESTS="$(
    curl -sf \
      "http://127.0.0.1:${LOKI_PORT}/api/debug/requests"
  )"; then
    break
  fi
  sleep 1
done

: "${REQUESTS:?mock Loki unreachable}"

python3 - "${REQUESTS}" <<'PY'
import json
import re
import sys

payload = json.loads(sys.argv[1])
requests = payload.get("requests", [])

if not requests:
    raise SystemExit("no Loki queries were recorded")

label_matcher = re.compile(
    r'([A-Za-z_][A-Za-z0-9_]*)\s*(=~|!~|!=|=)\s*"([^"]*)"'
)
selector_re = re.compile(r'^\s*\{([^}]*)\}')

storage_signals = (
    "129",
    "storport",
    "reset",
    "storage",
    "disk",
    "volume",
    "raidport",
    "scsi",
)

namespace_scoped_request = None
fallback_request = None

for request in requests:
    if request.get("path") != "/loki/api/v1/query_range":
        continue

    args = request.get("query", {})
    logql = args.get("query", "")

    selector_match = selector_re.search(logql)
    if not selector_match:
        continue

    selector = selector_match.group(1)

    matchers = {
        name.lower(): (operator, value)
        for name, operator, value in label_matcher.findall(selector)
    }

    lower = logql.lower()

    if not any(signal in lower for signal in storage_signals):
        continue

    limit_raw = args.get("limit")
    if limit_raw not in (None, ""):
        try:
            limit = int(limit_raw)
        except (TypeError, ValueError):
            continue

        if limit <= 0 or limit > 1000:
            continue

    namespace = matchers.get("namespace")

    # First prove the agent investigated the namespace requested by the user.
    if (
        namespace is not None
        and namespace[0] == "="
        and namespace[1] == "guest-observability-eval"
    ):
        namespace_scoped_request = request
        continue

    # Then require one bounded fallback capable of finding telemetry whose
    # namespace label is absent.
    if namespace is not None:
        continue

    # Keep the fallback selective instead of accepting an unrestricted scan.
    if not any(
        label in matchers
        for label in ("vm_name", "os", "source")
    ):
        continue

    fallback_request = request

if namespace_scoped_request is None:
    print("Recorded Loki requests:")
    print(json.dumps(requests, indent=2))
    raise SystemExit(
        "no valid namespace-scoped storage investigation query found "
        "for scenario event129-missing-namespace"
    )

if fallback_request is None:
    print("Recorded Loki requests:")
    print(json.dumps(requests, indent=2))
    raise SystemExit(
        "no valid bounded storage-reset fallback query without namespace "
        "was found for scenario event129-missing-namespace"
    )

print("Valid namespace-scoped storage investigation query found:")
print(json.dumps(namespace_scoped_request, indent=2))

print("Valid bounded fallback query without namespace found:")
print(json.dumps(fallback_request, indent=2))
PY
