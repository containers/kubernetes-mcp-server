#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"

PF_PID_FILE="${REPO_ROOT}/.guest-observability-loki-pf.pid"

if [ -f "${PF_PID_FILE}" ]; then
  PID="$(cat "${PF_PID_FILE}")"

  if kill -0 "${PID}" 2>/dev/null; then
    kill "${PID}" 2>/dev/null || true
  fi

  rm -f "${PF_PID_FILE}"
fi

kubectl delete namespace guest-observability-eval \
  --ignore-not-found=true

for ((i = 0; i < 120; i++)); do
  if ! kubectl get namespace guest-observability-eval \
    >/dev/null 2>&1; then
    echo "Namespace guest-observability-eval fully deleted"
    break
  fi

  sleep 1
done

if kubectl get namespace guest-observability-eval \
  >/dev/null 2>&1; then
  echo "timed out waiting for namespace guest-observability-eval to be deleted" >&2
  exit 1
fi
