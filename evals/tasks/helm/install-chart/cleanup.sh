#!/usr/bin/env bash
# Clean up the helm release and namespace
helm uninstall my-redis -n helm-install-test --ignore-not-found || true
kubectl delete namespace helm-install-test --ignore-not-found
