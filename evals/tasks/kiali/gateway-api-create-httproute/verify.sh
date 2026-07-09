#!/usr/bin/env bash
set -euo pipefail

NS="bookinfo"
NAME="eval-create-route"

if kubectl get httproute "$NAME" -n "$NS" >/dev/null 2>&1; then
  echo "Verified: HTTPRoute '$NAME' exists in namespace '$NS'."
else
  echo "HTTPRoute '$NAME' not found in namespace '$NS'."
  exit 1
fi
