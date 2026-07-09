#!/usr/bin/env bash
set -euo pipefail

kubectl delete inferencepool eval-list-pool -n bookinfo --ignore-not-found
