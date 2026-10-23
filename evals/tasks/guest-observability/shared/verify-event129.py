#!/usr/bin/env python3

import json
import os
import re
import sys
import time
import urllib.error
import urllib.request


QUERY_RANGE_PATH = "/loki/api/v1/query_range"
NAMESPACE = "guest-observability-eval"
TARGET_VM = "win-event-129"
WINDOWS_SOURCE = "windows_eventlog"
MAX_LIMIT = 1000

STORAGE_SIGNALS = (
    "129",
    "reset to device",
    "storage reset",
    "storport",
    "reset",
    "storage",
    "disk",
    "volume",
    "raidport",
    "scsi",
)

CONTRACT_LABELS = (
    "namespace",
    "vm_name",
    "os",
    "source",
)

LABEL_MATCHER = re.compile(
    r"""([A-Za-z_][A-Za-z0-9_]*)\s*
        (=~|!~|!=|=)\s*
        (?:"([^"]*)"|'([^']*)')
    """,
    re.VERBOSE,
)

SELECTOR_RE = re.compile(r"^\s*\{([^}]*)\}")


def load_requests():
    port = os.environ.get(
        "GUEST_OBSERVABILITY_LOKI_PORT",
        "3101",
    )
    url = f"http://127.0.0.1:{port}/api/debug/requests"

    last_error = None

    for _ in range(5):
        try:
            with urllib.request.urlopen(
                url,
                timeout=5,
            ) as response:
                payload = json.load(response)

            requests = payload.get("requests", [])
            if not requests:
                raise SystemExit(
                    "no Loki queries were recorded"
                )

            return requests

        except (
            urllib.error.URLError,
            ConnectionError,
            TimeoutError,
        ) as error:
            last_error = error
            time.sleep(1)

    raise SystemExit(
        f"mock Loki unreachable: {last_error}"
    )


def query_range_requests(requests):
    return [
        request
        for request in requests
        if request.get("path") == QUERY_RANGE_PATH
    ]


def query_args(request):
    return request.get("query", {})


def logql(request):
    return query_args(request).get("query", "")


def selector(logql_value):
    match = SELECTOR_RE.search(logql_value)
    if not match:
        return ""

    return match.group(1)


def matchers(logql_value):
    selector_value = selector(logql_value)
    result = {}

    for match in LABEL_MATCHER.finditer(selector_value):
        name = match.group(1).lower()
        operator = match.group(2)
        value = (
            match.group(3)
            if match.group(3) is not None
            else match.group(4)
        )
        result[name] = (operator, value)

    return result


def exact_match(matcher, expected):
    return (
        matcher is not None
        and matcher[0] == "="
        and matcher[1] == expected
    )


def has_storage_signal(logql_value):
    lower = logql_value.lower()

    return any(
        signal in lower
        for signal in STORAGE_SIGNALS
    )


def has_windows_identity(logql_value):
    lower = logql_value.lower()

    return (
        'os="windows"' in lower
        or "os='windows'" in lower
        or WINDOWS_SOURCE in lower
    )


def safe_limit(args):
    value = args.get("limit")

    if value in (None, ""):
        return True

    try:
        numeric = int(value)
    except (TypeError, ValueError):
        return False

    return 1 <= numeric <= MAX_LIMIT


def bounded_time(args):
    start = args.get("start", "")
    end = args.get("end", "")

    return bool(
        start
        and end
        and start != end
    )


def dump_failure(requests, message):
    print("Recorded Loki requests:")
    print(json.dumps(requests, indent=2))
    raise SystemExit(message)


def print_request(message, request):
    print(message)
    print(json.dumps(request, indent=2))


def verify_complete(requests):
    for request in query_range_requests(requests):
        args = query_args(request)
        query = logql(request)
        lower = query.lower()

        if (
            NAMESPACE in lower
            and has_windows_identity(query)
            and has_storage_signal(query)
            and bounded_time(args)
            and safe_limit(args)
        ):
            print_request(
                "Valid Windows Event 129 Loki query found:",
                request,
            )
            return

    dump_failure(
        requests,
        "no valid Windows Event 129 Loki query was recorded",
    )


def verify_missing_all_labels(requests):
    for request in query_range_requests(requests):
        args = query_args(request)
        query = logql(request)
        parsed = matchers(query)

        if not exact_match(
            parsed.get("collector"),
            "guest-agent",
        ):
            continue

        if any(
            label in parsed
            for label in CONTRACT_LABELS
        ):
            continue

        if not has_storage_signal(query):
            continue

        if not safe_limit(args):
            continue

        print_request(
            "Valid orphan storage-reset Loki query found:",
            request,
        )
        return

    dump_failure(
        requests,
        "no valid orphan storage-reset Loki query found for "
        "scenario event129-missing-all-labels",
    )


def verify_missing_namespace(requests):
    namespace_scoped_request = None
    fallback_request = None

    for request in query_range_requests(requests):
        args = query_args(request)
        query = logql(request)
        parsed = matchers(query)

        if not has_storage_signal(query):
            continue

        if not safe_limit(args):
            continue

        namespace = parsed.get("namespace")

        if exact_match(namespace, NAMESPACE):
            namespace_scoped_request = request
            continue

        if namespace is not None:
            continue

        if not any(
            label in parsed
            for label in ("vm_name", "os", "source")
        ):
            continue

        fallback_request = request

    if namespace_scoped_request is None:
        dump_failure(
            requests,
            "no valid namespace-scoped storage investigation query "
            "found for scenario event129-missing-namespace",
        )

    if fallback_request is None:
        dump_failure(
            requests,
            "no valid bounded storage-reset fallback query without "
            "namespace was found for scenario "
            "event129-missing-namespace",
        )

    print_request(
        "Valid namespace-scoped storage investigation query found:",
        namespace_scoped_request,
    )
    print_request(
        "Valid bounded fallback query without namespace found:",
        fallback_request,
    )


def verify_missing_os(requests):
    for request in query_range_requests(requests):
        query = logql(request)
        parsed = matchers(query)

        if not exact_match(
            parsed.get("namespace"),
            NAMESPACE,
        ):
            continue

        if parsed.get("os") is not None:
            continue

        has_target_vm = exact_match(
            parsed.get("vm_name"),
            TARGET_VM,
        )
        has_windows_source = exact_match(
            parsed.get("source"),
            WINDOWS_SOURCE,
        )

        if (
            has_target_vm
            or has_windows_source
            or has_storage_signal(query)
        ):
            print_request(
                "Valid missing-os Loki query found:",
                request,
            )
            return

    dump_failure(
        requests,
        "no valid contract-negative Loki query found "
        "for scenario event129-missing-os",
    )


def verify_missing_source(requests):
    for request in query_range_requests(requests):
        query = logql(request)
        parsed = matchers(query)

        if not exact_match(
            parsed.get("namespace"),
            NAMESPACE,
        ):
            continue

        if parsed.get("source") is not None:
            continue

        has_windows_os = exact_match(
            parsed.get("os"),
            "windows",
        )
        has_target_vm = exact_match(
            parsed.get("vm_name"),
            TARGET_VM,
        )

        if (
            has_windows_os
            or has_target_vm
            or has_storage_signal(query)
        ):
            print_request(
                "Valid missing-source Loki query found:",
                request,
            )
            return

    dump_failure(
        requests,
        "no valid contract-negative Loki query found "
        "for scenario event129-missing-source",
    )


def verify_missing_vm_name(requests):
    for request in query_range_requests(requests):
        args = query_args(request)
        query = logql(request)
        parsed = matchers(query)

        if not has_storage_signal(query):
            continue

        if not exact_match(
            parsed.get("namespace"),
            NAMESPACE,
        ):
            continue

        if "vm_name" in parsed:
            continue

        if not bounded_time(args):
            continue

        if not safe_limit(args):
            continue

        print_request(
            "Valid contract-negative Loki query found:",
            request,
        )
        return

    dump_failure(
        requests,
        "no valid contract-negative Loki query found for "
        "scenario event129-missing-vm-name",
    )


def verify_scenario(scenario, requests):
    verifiers = {
        "event129": verify_complete,
        "event129-missing-all-labels":
            verify_missing_all_labels,
        "event129-missing-namespace":
            verify_missing_namespace,
        "event129-missing-os":
            verify_missing_os,
        "event129-missing-source":
            verify_missing_source,
        "event129-missing-vm-name":
            verify_missing_vm_name,
    }

    verifier = verifiers.get(scenario)
    if verifier is None:
        raise SystemExit(
            f"unsupported scenario: {scenario}"
        )

    verifier(requests)


def main():
    if len(sys.argv) != 2:
        raise SystemExit(
            "usage: verify-event129.py <scenario>"
        )

    verify_scenario(
        sys.argv[1],
        load_requests(),
    )


if __name__ == "__main__":
    main()
