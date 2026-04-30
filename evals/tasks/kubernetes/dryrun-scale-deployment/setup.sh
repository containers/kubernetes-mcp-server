#!/usr/bin/env bash
# Create namespace and a deployment with initial replicas
kubectl delete namespace dryrun-scale-test --ignore-not-found
kubectl create namespace dryrun-scale-test
kubectl create deployment web-app --image=quay.io/nginx/nginx-unprivileged --replicas=1 -n dryrun-scale-test

# Wait for initial deployment to be ready
for i in {1..30}; do
    if kubectl get deployment web-app -n dryrun-scale-test -o jsonpath='{.status.availableReplicas}' | grep -q "1"; then
        exit 0
    fi
    sleep 2
done

echo "Setup failed for scale-deployment"
exit 1