#!/usr/bin/env bash
set -euo pipefail

GATEWAY_API_VERSION="${GATEWAY_API_VERSION:-v1.5.0}"

if kubectl get crd gateways.gateway.networking.k8s.io >/dev/null 2>&1; then
  echo "Gateway API CRDs already installed"
  exit 0
fi

echo "Installing Gateway API CRDs ${GATEWAY_API_VERSION}..."
kubectl kustomize "github.com/kubernetes-sigs/gateway-api/config/crd?ref=${GATEWAY_API_VERSION}" | kubectl apply -f -
