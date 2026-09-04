#!/usr/bin/env bash
set -euo pipefail

# Check if service has endpoints. EndpointSlice/Endpoints propagation is
# asynchronous, so poll for a bounded period instead of failing on the first
# empty read -- otherwise a correct fix can still be reported as failed if it
# hasn't converged yet.
endpoints=""
for i in $(seq 1 15); do
  endpoints=$(kubectl get endpoints nginx -n web -o jsonpath='{.subsets[0].addresses}' 2>/dev/null || true)
  if [[ -n "$endpoints" ]]; then
    break
  fi
  sleep 2
done
if [[ -z "$endpoints" ]]; then
  echo "Service nginx in namespace web has no endpoints"
  exit 1
fi

# Verify service can access the pod. Use a PSS-restricted-compatible probe pod:
# plain `kubectl run` with busybox is blocked on OpenShift (restricted:latest).
# Do not swallow real delete errors with `|| true` here: --ignore-not-found
# already makes a missing pod a no-op, so any remaining failure is a genuine
# API/transport error and we should fail closed rather than risk reusing a
# stale, already-Succeeded probe pod from a previous run.
kubectl delete pod -n web test-connection --ignore-not-found >/dev/null
cat <<'EOF' | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: test-connection
  namespace: web
spec:
  restartPolicy: Never
  securityContext:
    runAsNonRoot: true
    seccompProfile:
      type: RuntimeDefault
  containers:
  - name: test-connection
    image: quay.io/curl/curl:8.11.1
    command: ["curl", "-sf", "--max-time", "15", "http://nginx"]
    securityContext:
      allowPrivilegeEscalation: false
      capabilities:
        drop: ["ALL"]
      runAsNonRoot: true
      seccompProfile:
        type: RuntimeDefault
EOF

if ! kubectl wait -n web --for=jsonpath='{.status.phase}'=Succeeded pod/test-connection --timeout=180s; then
  echo "Service connection probe did not succeed"
  kubectl get pod -n web test-connection -o yaml || true
  kubectl delete pod -n web test-connection --ignore-not-found >/dev/null 2>&1 || true
  exit 1
fi

kubectl delete pod -n web test-connection --ignore-not-found >/dev/null 2>&1 || true
exit 0
