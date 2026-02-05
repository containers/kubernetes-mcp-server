#!/usr/bin/env bash
# Create a test namespace and install a sample helm release for the agent to discover
kubectl delete namespace helm-list-test --ignore-not-found
kubectl create namespace helm-list-test

# Install a helm release (don't wait for pods - we just need the release to exist)
helm install test-nginx oci://registry-1.docker.io/bitnamicharts/nginx --namespace helm-list-test
