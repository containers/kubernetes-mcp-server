#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${KUBECONFIG:-}" && -n "${MCP_EVAL_KUBECONFIG:-}" ]]; then
  export KUBECONFIG="${MCP_EVAL_KUBECONFIG}"
fi

ANNOTATION=$(kubectl get deployment payment-service -n ssa-test \
  -o jsonpath='{.spec.template.metadata.annotations.kubectl\.kubernetes\.io/restartedAt}')
if [ -z "$ANNOTATION" ]; then
  echo "FAIL: restartedAt annotation not found on pod template"
  exit 1
fi
echo "PASS: restartedAt annotation present: $ANNOTATION"

FAIL=0
for FIELD in \
  "resources.limits.memory" \
  "resources.limits.cpu" \
  "resources.requests.memory" \
  "resources.requests.cpu"; do
  VALUE=$(kubectl get deployment payment-service -n ssa-test \
    -o jsonpath="{.spec.template.spec.containers[0].${FIELD}}")
  if [ -z "$VALUE" ]; then
    echo "FAIL: ${FIELD} was removed (SSA field abandonment)"
    FAIL=1
  else
    echo "PASS: ${FIELD} preserved: ${VALUE}"
  fi
done
exit $FAIL
