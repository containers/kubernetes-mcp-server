#!/usr/bin/env bash
set -euo pipefail

: "${APPS_NAMESPACE:?APPS_NAMESPACE must be set}"
: "${APPS_FIXTURE:?APPS_FIXTURE must be set}"

kubectl create -f - <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: $APPS_NAMESPACE
  labels:
    evals.kubernetes-mcp-server/suite: core-apps
EOF
# mcpchecker does not run task cleanup if setup fails. Roll back only after
# this invocation has successfully created its namespace.
rollback() {
  local task_status=$?
  if [[ "$task_status" != 0 ]]; then
    bash "$(dirname "${BASH_SOURCE[0]}")/cleanup.sh" || true
  fi
  exit "$task_status"
}
trap rollback EXIT
case "$APPS_FIXTURE" in
  namespace) ;;
  pod)
    kubectl -n "$APPS_NAMESPACE" run app-demo --image=registry.k8s.io/pause:3.10 --labels=app=mcp-apps-eval
    kubectl -n "$APPS_NAMESPACE" wait --for=condition=Ready pod/app-demo --timeout=120s
    ;;
  *) echo "Unknown MCP Apps fixture: $APPS_FIXTURE" >&2; exit 1 ;;
esac
