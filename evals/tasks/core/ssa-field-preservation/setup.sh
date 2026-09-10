#!/usr/bin/env bash
set -euo pipefail

# Prefer an explicit kubeconfig when CI sets MCP_EVAL_KUBECONFIG but not KUBECONFIG.
# The mcpchecker kubernetes extension defaults to $HOME/.kube/config (often
# /alabama/.kube/config in CI) when KUBECONFIG is unset; shell setup uses kubectl
# and honors KUBECONFIG like the other core tasks.
if [[ -z "${KUBECONFIG:-}" && -n "${MCP_EVAL_KUBECONFIG:-}" ]]; then
  export KUBECONFIG="${MCP_EVAL_KUBECONFIG}"
fi

kubectl delete namespace ssa-test --ignore-not-found
kubectl create namespace ssa-test

# Apply a deployment with resource limits using SSA with the same field manager
# that resources_create_or_update uses, simulating a prior MCP tool call.
kubectl apply --server-side --field-manager=kubernetes-mcp-server --force-conflicts -f - <<'MANIFEST'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: payment-service
  namespace: ssa-test
  labels:
    app: payment-service
    team: platform
spec:
  replicas: 2
  selector:
    matchLabels:
      app: payment-service
  template:
    metadata:
      labels:
        app: payment-service
    spec:
      containers:
      - name: payment
        image: quay.io/nginx/nginx-unprivileged:latest
        ports:
        - containerPort: 8080
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi
MANIFEST

# Default matches generic K8s; slower environments can override via
# VERIFY_TIMEOUT without changing the default for everyone else.
kubectl wait --for=condition=Available deployment/payment-service -n ssa-test --timeout="${VERIFY_TIMEOUT:-120s}"
