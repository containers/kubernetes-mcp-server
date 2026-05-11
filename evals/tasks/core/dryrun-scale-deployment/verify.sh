#!/usr/bin/env bash
# Verify deployment scale out was not applied (dry run).
TIMEOUT="120s"

# Wait for deployment to be available
kubectl wait --for=condition=Available=True --timeout=$TIMEOUT deployment/web-app -n dryrun-scale-test
# Verify the replica count is still set to 1
if [ "$(kubectl get deployment web-app -n dryrun-scale-test -o jsonpath='{.status.availableReplicas}')" = "1" ]; then
    exit 0
fi

# If we get here, deployment didn't scale up correctly in time
echo "Verification failed for dryrun-scale-deployment"
exit 1 
