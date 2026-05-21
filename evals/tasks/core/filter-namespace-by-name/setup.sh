#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

kubectl delete namespace ns-alpha ns-beta ns-gamma --ignore-not-found
kubectl create namespace ns-alpha
kubectl create namespace ns-beta
kubectl create namespace ns-gamma
