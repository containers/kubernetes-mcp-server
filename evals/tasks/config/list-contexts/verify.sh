#!/usr/bin/env bash

# Verify the current kubeconfig has at least one context
CONTEXTS=$(kubectl config get-contexts -o name 2>/dev/null || echo "")

if [ -n "$CONTEXTS" ]; then
    echo "SUCCESS: Found contexts in kubeconfig:"
    echo "$CONTEXTS"
    exit 0
else
    echo "ERROR: No contexts found in kubeconfig"
    exit 1
fi
