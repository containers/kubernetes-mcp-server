#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

NAMESPACE=events-filter-test

kubectl delete namespace ${NAMESPACE} --ignore-not-found
kubectl create namespace ${NAMESPACE}

# Create a pod with a bad image to generate Warning events
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: bad-pod
  namespace: ${NAMESPACE}
spec:
  containers:
  - name: bad
    image: quay.io/this-image-does-not-exist:latest
    imagePullPolicy: Always
EOF

# Create a healthy pod to generate Normal events
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: good-pod
  namespace: ${NAMESPACE}
spec:
  containers:
  - name: good
    image: quay.io/nginx/nginx-unprivileged:latest
EOF

# Wait for the good-pod to be ready (will generate a Normal event)
kubectl wait --for=condition=Ready pod/good-pod -n ${NAMESPACE} --timeout=120s

# Wait for Warning events to appear from the bad image pod
for i in {1..30}; do
  if kubectl get events -n ${NAMESPACE} --field-selector=type=Warning --no-headers 2>/dev/null | grep -q "."; then
    exit 0
  fi
  sleep 2
done

echo "Warning events did not appear in time"
exit 1
