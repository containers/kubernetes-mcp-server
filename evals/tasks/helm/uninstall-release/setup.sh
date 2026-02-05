#!/usr/bin/env bash
# Create namespace and install a helm release that the agent will uninstall
kubectl delete namespace helm-uninstall-test --ignore-not-found
kubectl create namespace helm-uninstall-test

# Install a helm release (don't wait for pods - we just need the release to exist)
helm install cleanup-test oci://registry-1.docker.io/bitnamicharts/nginx --namespace helm-uninstall-test
