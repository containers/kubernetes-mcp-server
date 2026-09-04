#!/usr/bin/env bash
set -euo pipefail

# --- Configuration ---
NAMESPACE="statefulset-test"
STS_NAME="db"
EXPECTED_CONTENT="initial_data"
# Defaults match generic K8s; slower environments (e.g. cloud OCP PVC binding
# lag) can override via VERIFY_TIMEOUT without changing the default for
# everyone else.
DELETE_TIMEOUT="${VERIFY_TIMEOUT:-120s}"
READY_TIMEOUT="${VERIFY_TIMEOUT:-120s}"

echo "Verifying old pods are deleted"
# Wait for scale-down: deletion of db-1/db-2 (may already be gone).
# Fail closed: --ignore-not-found only suppresses the "not found" case, so any
# other API/auth/transport error still returns non-zero and is treated as a
# verification failure instead of being silently read as "pod is gone".
for pod in db-1 db-2; do
  kubectl wait "pod/${pod}" -n "${NAMESPACE}" --for=delete --timeout="${DELETE_TIMEOUT}" 2>/dev/null || true
  if ! out=$(kubectl get pod "$pod" -n "${NAMESPACE}" --ignore-not-found -o name 2>&1); then
    echo "Unable to verify pod $pod was deleted: $out"
    exit 1
  fi
  if [[ -n "$out" ]]; then
    echo "Pod $pod still exists after scale-down"
    exit 1
  fi
done
echo "Old pods are deleted"

# Verify correct number of replicas
echo "Verifying StatefulSet replica count"
replicas=$(kubectl get sts "${STS_NAME}" -n "${NAMESPACE}" -o jsonpath='{.spec.replicas}')
if [[ "${replicas}" -ne 1 ]]; then
  echo "Expected 1 replicas, but got $replicas"
  exit 1
fi
echo "StatefulSet is running with 1 replicas"

# On cloud OCP, PVC binding can lag; wait for db-0 Ready before reading data
echo "Waiting for pod db-0 to become Ready"
if ! kubectl wait --for=condition=Ready "pod/db-0" -n "${NAMESPACE}" --timeout="${READY_TIMEOUT}"; then
  echo "Pod db-0 not Ready in time"
  kubectl get pvc,pod -n "${NAMESPACE}" -o wide || true
  exit 1
fi

# Verify db-0 has the correct data
for pod in db-0; do
  if ! kubectl get pod "$pod" -n "${NAMESPACE}" &> /dev/null; then
    echo "Pod $pod not found in namespace $NAMESPACE"
    exit 1
  fi

  data=$(kubectl exec "$pod" -n "${NAMESPACE}" -- cat /data/test)
  if [[ "$data" != "${EXPECTED_CONTENT}" ]]; then
    echo "Data missing or incorrect in $pod"
    exit 1
  fi
done

exit 0
