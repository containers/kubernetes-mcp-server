#!/usr/bin/env bash
set -euo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
kubectl delete -f "${here}/signals.yaml" --ignore-not-found
"${here}/teardown.sh"
