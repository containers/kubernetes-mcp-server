#!/usr/bin/env bash
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
"${here}/setup.sh"
kubectl apply -f "${here}/signals.yaml"

# The target is intentionally not Ready; wait for Running and for both signals
# instead of waiting for its Deployment rollout.
for _ in $(seq 1 90); do
  pod="$(kubectl -n korrel8r-eval get pods -l app=checkout-eval -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
  if [[ -n "${pod}" ]]; then
    uid="$(kubectl -n korrel8r-eval get pod "${pod}" -o jsonpath='{.metadata.uid}')"
    phase="$(kubectl -n korrel8r-eval get pod "${pod}" -o jsonpath='{.status.phase}')"
    if [[ "${phase}" == Running ]] && \
       kubectl -n korrel8r-eval logs "${pod}" -c app 2>/dev/null | grep -qF 'checkout-eval: dependency unavailable'; then
      if kubectl -n korrel8r-eval get events --field-selector "involvedObject.uid=${uid},reason=Unhealthy" -o name 2>/dev/null | grep -q .; then
        # Verify that Korrel8r's direct store can retrieve the log, not just
        # that kubectl can. Use the same label selector the agent can discover.
        if curl --fail --silent --max-time 5 --get \
          --data-urlencode 'query=log:application:{"namespace":"korrel8r-eval","labels":{"app":"checkout-eval"},"containers":["app"]}' \
          --data-urlencode 'limit=10' \
          "http://127.0.0.1:${KORREL8R_EVAL_PORT:-18080}/api/v1alpha1/objects" | grep -qF 'checkout-eval: dependency unavailable'; then
          echo "Korrel8r signals ready for pod ${pod}"
          exit 0
        fi
      fi
    fi
  fi
  sleep 2
done
echo 'Timed out waiting for target Pod log, Unhealthy Event, and Korrel8r direct log query' >&2
kubectl -n korrel8r-eval get pods,events >&2
exit 1
