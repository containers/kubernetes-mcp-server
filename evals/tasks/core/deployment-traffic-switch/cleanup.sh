#!/bin/bash
set -e
NAMESPACE="e-commerce"

# Delete the namespace to clean up all resources created during the evaluation
echo "Deleting namespace '$NAMESPACE'..."
kubectl delete namespace $NAMESPACE --wait=false
