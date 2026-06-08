#!/usr/bin/env bash
# Idempotent NetObserv mock plugin setup for mcpchecker tasks.
# Deploys the in-cluster mock API and ensures localhost:9001 is reachable.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"
MANIFEST="${SCRIPT_DIR}/mock-plugin.yaml"
NETOBSERV_PORT="${NETOBSERV_PORT:-9001}"
PF_PID_FILE="${PF_PID_FILE:-${REPO_ROOT}/.netobserv-pf.pid}"

kubectl apply -f "${MANIFEST}"
kubectl wait --namespace netobserv --for=condition=available deployment/netobserv-plugin --timeout=120s

if curl -sf "http://127.0.0.1:${NETOBSERV_PORT}/api/resources/namespaces" >/dev/null 2>&1; then
	echo "NetObserv mock already reachable on http://127.0.0.1:${NETOBSERV_PORT}"
	exit 0
fi

if [ -f "${PF_PID_FILE}" ]; then
	OLD_PID="$(cat "${PF_PID_FILE}")"
	if kill -0 "${OLD_PID}" 2>/dev/null; then
		kill "${OLD_PID}" 2>/dev/null || true
	fi
	rm -f "${PF_PID_FILE}"
fi

kubectl -n netobserv port-forward svc/netobserv-plugin "${NETOBSERV_PORT}:9001" >/dev/null 2>&1 &
echo $! > "${PF_PID_FILE}"

for _ in $(seq 1 30); do
	if curl -sf "http://127.0.0.1:${NETOBSERV_PORT}/api/resources/namespaces" >/dev/null 2>&1; then
		echo "NetObserv mock ready at http://127.0.0.1:${NETOBSERV_PORT}"
		exit 0
	fi
	sleep 1
done

echo "Timed out waiting for NetObserv mock on http://127.0.0.1:${NETOBSERV_PORT}"
exit 1
