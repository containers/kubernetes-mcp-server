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
import sys

payload = json.loads(sys.argv[1])
requests = payload.get("requests", [])

if not requests:
    raise SystemExit("no Loki queries were recorded")

valid_request = None

for request in requests:
    if request.get("path") != "/loki/api/v1/query_range":
        continue

    args = request.get("query", {})

    logql = args.get("query", "")
    start = args.get("start", "")
    end = args.get("end", "")
    limit = args.get("limit")

    lower = logql.lower()

    has_namespace = (
        "guest-observability-eval" in lower
    )

    has_windows_identity = (
        'os="windows"' in lower
        or "os='windows'" in lower
        or "windows_eventlog" in lower
    )

    has_storage_reset_filter = any(
        token in lower
        for token in (
            "129",
            "reset to device",
            "storage reset",
            "storport",
            "storage",
            "disk",
            "reset",
        )
    )

    bounded_time = bool(
        start
        and end
        and start != end
    )

    safe_limit = True

    if limit not in (None, ""):
        try:
            numeric_limit = int(limit)
            safe_limit = 1 <= numeric_limit <= 1000
        except (TypeError, ValueError):
            safe_limit = False

    if (
        has_namespace
        and has_windows_identity
        and has_storage_reset_filter
        and bounded_time
        and safe_limit
    ):
        valid_request = request
        break

if valid_request is None:
    print("Recorded Loki requests:")
    print(json.dumps(requests, indent=2))

    raise SystemExit(
        "no valid Windows Event 129 Loki query was recorded"
    )

print("Valid Windows Event 129 Loki query found:")
print(json.dumps(valid_request, indent=2))
PY
