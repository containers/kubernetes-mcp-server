#!/usr/bin/env bash
# Prepare the real API-only Korrel8r backend for an eval run (idempotent).
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "${here}/../../../.." && pwd)"
port="${KORREL8R_EVAL_PORT:-18080}"
pid_file="${root}/_output/korrel8r-eval-pf.pid"
endpoint="http://127.0.0.1:${port}/api/v1alpha1/domains"

# The deployed Korrel8r instance is also the correlation fixture: its
# Deployment, ReplicaSet and Pod exist on any cluster that can run the eval.
kubectl apply -f "${here}/korrel8r.yaml"
kubectl -n korrel8r-eval rollout status deployment/korrel8r --timeout=180s

# Do not accept an unrelated server already bound to the configured port.
if [[ -f "${pid_file}" ]] && kill -0 "$(<"${pid_file}")" 2>/dev/null; then
  if curl --fail --silent --max-time 2 "${endpoint}" | grep -q '"k8s"'; then
    exit 0
  fi
  old_pid="$(<"${pid_file}")"
  kill "${old_pid}" 2>/dev/null || true
  # Wait for the old port-forward to release the port before rebinding it.
  for _ in $(seq 1 10); do
    kill -0 "${old_pid}" 2>/dev/null || break
    sleep 0.5
  done
  kill -9 "${old_pid}" 2>/dev/null || true
fi
mkdir -p "$(dirname "${pid_file}")"
kubectl -n korrel8r-eval port-forward --address 127.0.0.1 deployment/korrel8r "${port}:8080" > "${root}/_output/korrel8r-eval-pf.log" 2>&1 &
echo $! > "${pid_file}"
for _ in $(seq 1 30); do
  if curl --fail --silent --max-time 2 "${endpoint}" | grep -q '"k8s"'; then
    echo "Korrel8r API ready on localhost:${port}"
    exit 0
  fi
  if ! kill -0 "$(<"${pid_file}")" 2>/dev/null; then
    break
  fi
  sleep 1
done
printf 'Korrel8r API not ready; port-forward log:\n' >&2
# Log is printed to help distinguish image/startup/RBAC/port conflicts.
tail -n 30 "${root}/_output/korrel8r-eval-pf.log" >&2
exit 1
