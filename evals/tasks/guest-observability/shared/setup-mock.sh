#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"

NAMESPACE="guest-observability-eval"
DEPLOYMENT="mock-loki"
APP_LABEL="app=mock-loki"

LOCAL_PORT="3101"
TARGET_PORT="3100"

SCENARIO="${GUEST_OBSERVABILITY_SCENARIO:-event129}"

MANIFEST="${SCRIPT_DIR}/mock-loki.yaml"
PID_FILE="${REPO_ROOT}/.guest-observability-loki-pf.pid"
PF_LOG="/tmp/guest-observability-loki-pf.log"

BASE_URL="http://127.0.0.1:${LOCAL_PORT}"

ROLLOUT_ID="$(
  python3 -c 'import uuid; print(uuid.uuid4())'
)"

RENDERED_MANIFEST="$(
  mktemp "${TMPDIR:-/tmp}/guest-observability-mock-loki.XXXXXX"
)"

cleanup_temp() {
  rm -f "${RENDERED_MANIFEST}"
}

trap cleanup_temp EXIT


echo "Setting up Mock Loki"
echo "Scenario: ${SCENARIO}"
echo "Rollout ID: ${ROLLOUT_ID}"


#
# 1. Render one final Deployment configuration.
#
# mock-loki.yaml contains:
#
#   - name: GUEST_OBSERVABILITY_SCENARIO
#     value: event129
#
# Replace that value in a temporary manifest with the requested
# scenario.
#
# We also add MOCK_LOKI_ROLLOUT_ID. Because its value changes on
# every setup invocation, Kubernetes creates one fresh pod even
# when:
#
#   - the scenario is unchanged
#   - only ConfigMap/server.py changed
#
# This means we do NOT need:
#
#   kubectl set env
#   kubectl rollout restart
#
# and therefore avoid multiple back-to-back ReplicaSets.
#
python3 - \
  "${MANIFEST}" \
  "${RENDERED_MANIFEST}" \
  "${SCENARIO}" \
  "${ROLLOUT_ID}" <<'PY'
from pathlib import Path
import json
import sys

source_path = Path(sys.argv[1])
output_path = Path(sys.argv[2])
scenario = sys.argv[3]
rollout_id = sys.argv[4]

text = source_path.read_text()

needle = """          env:
            - name: GUEST_OBSERVABILITY_SCENARIO
              value: event129
"""

replacement = f"""          env:
            - name: GUEST_OBSERVABILITY_SCENARIO
              value: {json.dumps(scenario)}
            - name: MOCK_LOKI_ROLLOUT_ID
              value: {json.dumps(rollout_id)}
"""

count = text.count(needle)

if count != 1:
    raise SystemExit(
        "ERROR: expected exactly one "
        "GUEST_OBSERVABILITY_SCENARIO env block in "
        f"{source_path}, found {count}"
    )

output_path.write_text(
    text.replace(
        needle,
        replacement,
        1,
    )
)
PY


#
# 2. Apply everything exactly once.
#
# This updates:
#
#   Namespace
#   ConfigMap
#   Deployment
#   Service
#
# The Deployment receives both the requested scenario and the unique
# rollout ID in this single apply.
#
kubectl apply -f "${RENDERED_MANIFEST}"


#
# 3. Wait for the Deployment rollout.
#
kubectl rollout status \
  --namespace "${NAMESPACE}" \
  "deployment/${DEPLOYMENT}" \
  --timeout=120s


#
# 4. Wait until Kubernetes has exactly one Mock Loki pod.
#
# rollout status can succeed while an old pod is still Terminating.
# Do not continue until:
#
#   - exactly one matching pod exists
#   - it is not terminating
#   - phase is Running
#   - Ready=True
#
POD=""

for _ in $(seq 1 120); do
  POD="$(
    kubectl get pods \
      --namespace "${NAMESPACE}" \
      --selector "${APP_LABEL}" \
      -o json \
    | python3 -c '
import json
import sys

data = json.load(sys.stdin)
pods = data.get("items", [])

if len(pods) != 1:
    sys.exit(0)

pod = pods[0]

metadata = pod.get("metadata", {})
status = pod.get("status", {})

if metadata.get("deletionTimestamp"):
    sys.exit(0)

if status.get("phase") != "Running":
    sys.exit(0)

ready = any(
    condition.get("type") == "Ready"
    and condition.get("status") == "True"
    for condition in status.get("conditions", [])
)

if not ready:
    sys.exit(0)

name = metadata.get("name")

if name:
    print(name)
'
  )"

  if [ -n "${POD}" ]; then
    break
  fi

  sleep 1
done


if [ -z "${POD}" ]; then
  echo
  echo "ERROR: Mock Loki did not converge to exactly one Ready pod"
  echo

  kubectl get pods \
    --namespace "${NAMESPACE}" \
    --selector "${APP_LABEL}" \
    -o wide

  exit 1
fi


echo "Using Mock Loki pod: ${POD}"


#
# 5. Verify that the final pod is running exactly the configuration
# we requested.
#
POD_SCENARIO="$(
  kubectl exec \
    --namespace "${NAMESPACE}" \
    "${POD}" \
    -- printenv GUEST_OBSERVABILITY_SCENARIO
)"

POD_ROLLOUT_ID="$(
  kubectl exec \
    --namespace "${NAMESPACE}" \
    "${POD}" \
    -- printenv MOCK_LOKI_ROLLOUT_ID
)"


if [ "${POD_SCENARIO}" != "${SCENARIO}" ]; then
  echo "ERROR: pod scenario mismatch"
  echo "Expected: ${SCENARIO}"
  echo "Actual:   ${POD_SCENARIO}"
  exit 1
fi


if [ "${POD_ROLLOUT_ID}" != "${ROLLOUT_ID}" ]; then
  echo "ERROR: pod rollout ID mismatch"
  echo "Expected: ${ROLLOUT_ID}"
  echo "Actual:   ${POD_ROLLOUT_ID}"
  exit 1
fi


echo "Mock Loki scenario verified: ${POD_SCENARIO}"
echo "Mock Loki rollout verified: ${POD_ROLLOUT_ID}"


#
# 6. Stop the previous port-forward recorded by this test harness.
#
if [ -f "${PID_FILE}" ]; then
  OLD_PID="$(
    cat "${PID_FILE}" 2>/dev/null || true
  )"

  if [ -n "${OLD_PID}" ] \
    && kill -0 "${OLD_PID}" 2>/dev/null; then

    OLD_COMMAND="$(
      ps -p "${OLD_PID}" -o command= 2>/dev/null || true
    )"

    case "${OLD_COMMAND}" in
      *kubectl*port-forward*)
        echo "Stopping old Mock Loki port-forward PID ${OLD_PID}"
        kill "${OLD_PID}" 2>/dev/null || true
        ;;
    esac
  fi

  rm -f "${PID_FILE}"
fi


#
# 7. Remove any orphan Mock Loki port-forward whose PID file was lost.
#
pkill -u "$(id -u)" -f \
  "kubectl.*port-forward.*${LOCAL_PORT}:${TARGET_PORT}" \
2>/dev/null || true


#
# 8. Wait until no process is accepting connections on LOCAL_PORT.
#
port_has_listener() {
  python3 - "${LOCAL_PORT}" <<'PY'
import socket
import sys

port = int(sys.argv[1])

sock = socket.socket(
    socket.AF_INET,
    socket.SOCK_STREAM,
)

sock.settimeout(0.2)

try:
    result = sock.connect_ex(
        ("127.0.0.1", port)
    )
finally:
    sock.close()

sys.exit(0 if result == 0 else 1)
PY
}


for _ in $(seq 1 20); do
  if ! port_has_listener; then
    break
  fi

  sleep 1
done


if port_has_listener; then
  echo "ERROR: local port ${LOCAL_PORT} is still in use"
  exit 1
fi


#
# 9. Start a fresh port-forward directly against the exact pod that
# passed all of our rollout checks.
#
rm -f "${PF_LOG}"

nohup kubectl port-forward \
  --namespace "${NAMESPACE}" \
  "pod/${POD}" \
  "${LOCAL_PORT}:${TARGET_PORT}" \
  >"${PF_LOG}" 2>&1 &

PF_PID=$!

echo "${PF_PID}" > "${PID_FILE}"

echo "Started Mock Loki port-forward PID ${PF_PID}"


#
# 10. Verify the new port-forward process itself stays alive and the
# selected Mock Loki pod answers /ready.
#
READY="false"

for _ in $(seq 1 60); do
  if ! kill -0 "${PF_PID}" 2>/dev/null; then
    echo
    echo "ERROR: Mock Loki port-forward exited unexpectedly"
    echo

    cat "${PF_LOG}" 2>/dev/null || true

    rm -f "${PID_FILE}"

    exit 1
  fi

  if curl -fsS \
      "${BASE_URL}/ready" \
      >/dev/null 2>&1; then

    READY="true"
    break
  fi

  sleep 1
done


if [ "${READY}" != "true" ]; then
  echo
  echo "ERROR: Mock Loki did not become ready at ${BASE_URL}"
  echo

  cat "${PF_LOG}" 2>/dev/null || true

  kill "${PF_PID}" 2>/dev/null || true
  rm -f "${PID_FILE}"

  exit 1
fi


echo "Mock Loki ready at ${BASE_URL}"


#
# 11. Reset request history only after the final pod and fresh
# port-forward are confirmed healthy.
#
curl -fsS \
  "${BASE_URL}/api/debug/reset" \
  >/dev/null


echo "Mock Loki request history reset"
