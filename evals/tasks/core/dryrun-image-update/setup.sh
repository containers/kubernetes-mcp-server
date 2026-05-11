#!/usr/bin/env bash
# Create namespace and a deployment referencing image tag 1.24. 
kubectl delete namespace dryrun-image-update --ignore-not-found
kubectl create namespace dryrun-image-update
kubectl create deployment web-app --image=quay.io/nginx/nginx-unprivileged:1.24 --replicas=1 -n dryrun-image-update

# Wait until all replicas are available
TIMEOUT="120s"
if kubectl wait deployment/web-app -n dryrun-image-update --for=condition=Available=True --timeout=$TIMEOUT; then
  echo "Setup succeeded for dryrun-image-update"
  exit 0
else
  echo "Setup failed for dryrun-image-update. Initial deployment did not become ready in time"
  exit 1
fi