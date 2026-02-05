#!/usr/bin/env bash
# Clean up the test namespace and helm release
helm uninstall test-nginx -n helm-list-test --ignore-not-found || true
kubectl delete namespace helm-list-test --ignore-not-found
