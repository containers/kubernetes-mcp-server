#!/usr/bin/env bash

# Verify the kubeconfig has a current context
CURRENT_CONTEXT=$(kubectl config current-context 2>/dev/null || echo "")
CONTEXTS=$(kubectl config get-contexts -o name 2>/dev/null || echo "")

if [ -n "$CURRENT_CONTEXT" ]; then
    echo "SUCCESS: Found current context: $CURRENT_CONTEXT"

    # Check if there are any contexts at all
    CONTEXT_COUNT=$(echo "$CONTEXTS" | wc -l)
    echo "Total contexts available: $CONTEXT_COUNT"
    echo "$CONTEXTS"
    exit 0
else
    echo "ERROR: No current context found"
    exit 1
fi
