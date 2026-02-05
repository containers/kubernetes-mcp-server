#!/usr/bin/env bash
# Create a clean namespace for the helm install test
kubectl delete namespace helm-install-test --ignore-not-found
kubectl create namespace helm-install-test
