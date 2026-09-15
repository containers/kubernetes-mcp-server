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
    source = matchers.get("source")
    os_label = matchers.get("os")
    vm_name = matchers.get("vm_name")

    if namespace is None:
        continue

    if namespace[0] != "=":
        continue

    if namespace[1] != "guest-observability-eval":
        continue

    #
    # The contract-negative Event 129 stream intentionally has no
    # "source" label. A discovery query must therefore not require it.
    #
    if source is not None:
        continue

    has_windows_os = (
        os_label is not None
        and os_label[0] == "="
        and os_label[1] == "windows"
    )

    has_target_vm = (
        vm_name is not None
        and vm_name[0] == "="
        and vm_name[1] == "win-event-129"
    )

    lower = logql.lower()

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
    # The query only needs to be capable of exposing the stream whose
    # source label is intentionally missing while retaining useful
    # remaining identity/classification evidence.
    #
    if (
        has_windows_os
        or has_target_vm
        or has_event_signal
    ):
        valid_request = request
        break

if valid_request is None:
    print("Recorded Loki requests:")
    print(json.dumps(requests, indent=2))

    raise SystemExit(
        "no valid contract-negative Loki query found "
        "for scenario event129-missing-source"
    )

print("Valid missing-source Loki query found:")
print(json.dumps(valid_request, indent=2))
PY
