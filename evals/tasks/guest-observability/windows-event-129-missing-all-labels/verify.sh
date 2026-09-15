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

valid_request = None

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

    collector = matchers.get("collector")

    # The orphan stream can be discovered through the known ingestion label.
    if collector is None:
        continue

    if collector[0] != "=" or collector[1] != "guest-agent":
        continue

    # None of the guest telemetry contract labels exist on this stream.
    forbidden_contract_labels = {
        "namespace",
        "vm_name",
        "os",
        "source",
    }

    if forbidden_contract_labels.intersection(matchers):
        continue

    lower = logql.lower()

    # Accept different model-generated storage-reset searches rather than
    # requiring one exact Event 129 query shape.
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

    valid_request = request
    break

if valid_request is None:
    print("Recorded Loki requests:")
    print(json.dumps(requests, indent=2))
    raise SystemExit(
        "no valid orphan storage-reset Loki query found for "
        "scenario event129-missing-all-labels"
    )

print("Valid orphan storage-reset Loki query found:")
print(json.dumps(valid_request, indent=2))
PY
