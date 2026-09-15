#!/usr/bin/env python3

import json
import os
import sys
import urllib.request


if len(sys.argv) != 2:
    raise SystemExit(
        "usage: verify-contract-negative.py <scenario>"
    )

scenario = sys.argv[1]

port = os.environ.get(
    "GUEST_OBSERVABILITY_LOKI_PORT",
    "3101",
)

url = (
    f"http://127.0.0.1:{port}"
    "/api/debug/requests"
)

with urllib.request.urlopen(url, timeout=5) as response:
    payload = json.load(response)

requests = payload.get("requests", [])

if not requests:
    raise SystemExit("no Loki queries were recorded")


def selector(logql):
    if "{" not in logql or "}" not in logql:
        return ""

    return (
        logql
        .split("{", 1)[1]
        .split("}", 1)[0]
        .lower()
    )


def has_event_filter(lower):
    return any(
        token in lower
        for token in (
            "129",
            "reset to device",
            "storage reset",
            "storport",
            "reset",
        )
    )


def safe_limit(args):
    value = args.get("limit")

    if value is None:
        return True

    try:
        value = int(value)
    except ValueError:
        return False

    return 1 <= value <= 1000


def bounded(args):
    start = args.get("start", "")
    end = args.get("end", "")

    return bool(
        start
        and end
        and start != end
    )


def matches_scenario(logql):
    lower = logql.lower()
    sel = selector(logql)

    if not has_event_filter(lower):
        return False

    if scenario == "event129-missing-vm-name":
        return (
            'namespace="guest-observability-eval"' in sel
            and "vm_name" not in sel
        )

    if scenario == "event129-missing-namespace":
        return (
            (
                'os="windows"' in lower
                or "windows_eventlog" in lower
            )
            and "namespace" not in sel
        )

    if scenario == "event129-missing-os":
        return (
            "windows_eventlog" in lower
            and "os" not in sel
        )

    if scenario == "event129-missing-source":
        return (
            "guest-observability-eval" in lower
            and "win-event-129" in lower
            and 'os="windows"' in lower
            and "source" not in sel
        )

    if scenario == "event129-missing-all-labels":
        contract_labels = (
            "namespace",
            "vm_name",
            "os",
            "source",
        )

        return (
            'collector="guest-agent"' in lower
            and all(
                label not in sel
                for label in contract_labels
            )
        )

    raise SystemExit(
        f"unsupported scenario: {scenario}"
    )


valid = None

for request in requests:
    if request.get("path") != "/loki/api/v1/query_range":
        continue

    args = request.get("query", {})
    logql = args.get("query", "")

    if (
        matches_scenario(logql)
        and bounded(args)
        and safe_limit(args)
    ):
        valid = request
        break


if valid is None:
    print("Recorded Loki requests:")
    print(json.dumps(requests, indent=2))

    raise SystemExit(
        "no valid contract-negative Loki query "
        f"found for scenario {scenario}"
    )


print(
    "Valid contract-negative Loki query found:"
)
print(json.dumps(valid, indent=2))
