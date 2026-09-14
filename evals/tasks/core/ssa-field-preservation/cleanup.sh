#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${KUBECONFIG:-}" && -n "${MCP_EVAL_KUBECONFIG:-}" ]]; then
  export KUBECONFIG="${MCP_EVAL_KUBECONFIG}"
fi

kubectl delete namespace ssa-test --ignore-not-found
