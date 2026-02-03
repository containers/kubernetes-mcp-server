#!/usr/bin/env bash

# Verify the current kubeconfig has a valid structure
CURRENT_CONTEXT=$(kubectl config current-context 2>/dev/null || echo "")

if [ -n "$CURRENT_CONTEXT" ]; then
    CLUSTER_SERVER=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}' 2>/dev/null || echo "")
    echo "SUCCESS: Kubeconfig has valid structure"
    echo "Current context: $CURRENT_CONTEXT"
    echo "Cluster server: $CLUSTER_SERVER"
    exit 0
else
    echo "ERROR: No current context found in kubeconfig"
    exit 1
fi
