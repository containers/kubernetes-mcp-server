#!/usr/bin/env bash
set -euo pipefail

: "${APPS_NAMESPACE:?APPS_NAMESPACE must be set}"
# Do not remove a pre-existing namespace if setup failed to create our fixture.
namespace=$(kubectl get namespace "$APPS_NAMESPACE" --ignore-not-found -o json)
if [[ -z "$namespace" ]]; then
  exit 0
fi
owner=$(jq -r '.metadata.labels["evals.kubernetes-mcp-server/suite"]' <<< "$namespace")
if [[ "$owner" != core-apps ]]; then
  exit 0
fi

namespace_uid=$(jq -er '.metadata.uid | select(type == "string" and length > 0)' <<< "$namespace")
# The API server must reject deletion if the name now belongs to another UID.
jq -n --arg uid "$namespace_uid" '{apiVersion: "v1", kind: "DeleteOptions", preconditions: {uid: $uid}}' |
  kubectl delete --raw "/api/v1/namespaces/$APPS_NAMESPACE" -f - --request-timeout=30s

# Wait for this UID to disappear, without waiting on a replacement namespace.
deadline=$((SECONDS + 120))
while (( SECONDS < deadline )); do
  current_uid=$(kubectl get namespace "$APPS_NAMESPACE" --ignore-not-found -o jsonpath='{.metadata.uid}' --request-timeout=10s)
  if [[ "$current_uid" != "$namespace_uid" ]]; then
    exit 0
  fi
  sleep 1
done
echo "Timed out waiting for namespace $APPS_NAMESPACE ($namespace_uid) to be deleted" >&2
exit 1
