#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
"${SCRIPT_DIR}/../scripts/ensure_gateway_api_crds.sh"

cat <<'EOF' | kubectl apply -f -
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: eval-gateway
  namespace: bookinfo
  labels:
    gevals.kiali.io/test: gevals-testing
spec:
  gatewayClassName: istio
  listeners:
  - name: http
    port: 80
    protocol: HTTP
    allowedRoutes:
      namespaces:
        from: Same
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: eval-list-route
  namespace: bookinfo
  labels:
    gevals.kiali.io/test: gevals-testing
spec:
  parentRefs:
  - name: eval-gateway
    namespace: bookinfo
  rules:
  - matches:
    - path:
        type: PathPrefix
        value: /
    backendRefs:
    - name: productpage
      port: 9080
EOF
