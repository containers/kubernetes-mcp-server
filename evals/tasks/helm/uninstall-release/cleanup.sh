#!/usr/bin/env bash
# Clean up the namespace
# The helm release should already be uninstalled by the agent, but we'll try anyway just in case
helm uninstall cleanup-test -n helm-uninstall-test --ignore-not-found || true
kubectl delete namespace helm-uninstall-test --ignore-not-found
