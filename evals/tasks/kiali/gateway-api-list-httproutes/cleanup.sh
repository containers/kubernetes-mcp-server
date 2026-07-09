#!/usr/bin/env bash
set -euo pipefail

kubectl delete httproute eval-list-route -n bookinfo --ignore-not-found
kubectl delete gateway eval-gateway -n bookinfo --ignore-not-found
