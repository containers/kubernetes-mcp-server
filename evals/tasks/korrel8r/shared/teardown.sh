#!/usr/bin/env bash
# Remove the local Korrel8r eval backend and its port-forward.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "${here}/../../../.." && pwd)"
pid_file="${root}/_output/korrel8r-eval-pf.pid"

if [[ -f "${pid_file}" ]]; then
  kill "$(<"${pid_file}")" 2>/dev/null || true
  rm -f "${pid_file}"
fi
kubectl delete -f "${here}/korrel8r.yaml" --ignore-not-found
