#!/usr/bin/env bash
set -euo pipefail

INFERENCE_API_VERSION="${INFERENCE_API_VERSION:-v1.5.0}"

if kubectl get crd inferencepools.inference.networking.k8s.io >/dev/null 2>&1; then
  echo "Inference API CRDs already installed"
  exit 0
fi

echo "Installing Inference API CRDs ${INFERENCE_API_VERSION}..."
kubectl kustomize "github.com/kubernetes-sigs/gateway-api-inference-extension/config/crd?ref=${INFERENCE_API_VERSION}" | kubectl apply -f -
