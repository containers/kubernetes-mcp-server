#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
"${SCRIPT_DIR}/../scripts/ensure_inference_api_crds.sh"

cat <<'EOF' | kubectl apply -f -
apiVersion: inference.networking.k8s.io/v1
kind: InferencePool
metadata:
  name: eval-list-pool
  namespace: bookinfo
  labels:
    gevals.kiali.io/test: gevals-testing
spec:
  targetPorts:
  - number: 8000
  selector:
    matchLabels:
      app: example-model
  endpointPickerRef:
    group: ""
    kind: Service
    name: example-epp
    port:
      number: 9002
    failureMode: FailClose
EOF
