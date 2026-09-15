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

valid_request = None

for request in requests:
    if request.get("path") != "/loki/api/v1/query_range":
        continue

    args = request.get("query", {})
    logql = args.get("query", "")

    matchers = {
        name.lower(): (operator, value)
        for name, operator, value in label_matcher.findall(logql)
    }

    namespace = matchers.get("namespace")
    os_label = matchers.get("os")
    vm_name = matchers.get("vm_name")
    source = matchers.get("source")

    if namespace is None:
        continue

    if namespace[0] != "=":
        continue

    if namespace[1] != "guest-observability-eval":
        continue

    #
    # The contract-negative Event 129 stream intentionally has no
    # "os" label. A discovery query must therefore not require os.
    #
    if os_label is not None:
        continue

    lower = logql.lower()

    has_target_vm = (
        vm_name is not None
        and vm_name[0] == "="
        and vm_name[1] == "win-event-129"
    )

    has_windows_source = (
        source is not None
        and source[0] == "="
        and source[1] == "windows_eventlog"
    )

    has_event_signal = any(
        token in lower
        for token in (
            "129",
            "storport",
            "reset",
            "raidport",
            "storage",
            "disk",
        )
    )

    #
    # Do not require one exact LogQL selector shape.
    #
    # A query is valid if it:
    #   - stays scoped to the requested namespace,
    #   - does not require the intentionally missing os label, and
    #   - is narrowed by remaining contract information or the
    #     storage-reset signal itself.
    #
    if (
        has_target_vm
        or has_windows_source
        or has_event_signal
    ):
        valid_request = request
        break

if valid_request is None:
    print("Recorded Loki requests:")
    print(json.dumps(requests, indent=2))

    raise SystemExit(
        "no valid contract-negative Loki query found "
        "for scenario event129-missing-os"
    )

print("Valid missing-os Loki query found:")
print(json.dumps(valid_request, indent=2))
PY
