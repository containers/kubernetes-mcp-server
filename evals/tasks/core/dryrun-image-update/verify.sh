#!/usr/bin/env bash
TIMEOUT="120s"

# Wait for the deployment to become "Available"
kubectl wait --for=condition=Available deployment/web-app -n dryrun-image-update --timeout=$TIMEOUT

# There should be exactly 1 ReplicaSet
REPLICA_SET_COUNT=$(kubectl -n dryrun-image-update get rs -l app=web-app -o go-template='{{printf "%d\n" (len  .items)}}')
if [ "$REPLICA_SET_COUNT" -ne 1 ]; then
    echo "Verification failed: There should be exactly 1 ReplicaSet, but found $REPLICA_SET_COUNT."
    exit 1
fi

# There should be exactly 1 Pod running
POD_COUNT=$(kubectl -n dryrun-image-update get pods -l app=web-app --field-selector=status.phase=Running -o go-template='{{printf "%d\n" (len  .items)}}')
if [ "$POD_COUNT" -ne 1 ]; then
    echo "Verification failed: There should be exactly 1 running Pod, but found $POD_COUNT."
    exit 1
fi

# The running Pod must run the old image
POD_IMAGE=$(kubectl -n dryrun-image-update get pods -l app=web-app --field-selector=status.phase=Running -o jsonpath='{.items[0].spec.containers[0].image}')
if [ "$POD_IMAGE" != "quay.io/nginx/nginx-unprivileged:1.24" ]; then
    echo "Verification failed: The running Pod must run the old image 'quay.io/nginx/nginx-unprivileged:1.24', but found '$POD_IMAGE'."
    exit 1
fi

# If we get here, all verifications passed
echo "All verifications passed."
exit 0
