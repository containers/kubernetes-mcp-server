#!/usr/bin/env bash
# Verify that the helm release was installed
TIMEOUT=300

# Wait a bit for the agent to complete the installation
sleep 5

# CRITICAL: Check if the helm release exists
# This is the primary verification - we MUST have a helm release, not just pods
if ! helm list -n helm-install-test 2>/dev/null | grep -q "my-redis"; then
    echo "VERIFICATION FAILED: my-redis helm release not found"
    echo ""
    echo "Expected: A Helm release named 'my-redis' in namespace 'helm-install-test'"
    echo "Found helm releases in helm-install-test namespace:"
    helm list -n helm-install-test 2>/dev/null || echo "  (none)"
    echo ""
    echo "Note: This test requires using helm_install MCP tool to create a Helm release."
    exit 1
fi

echo "✓ Helm release 'my-redis' found successfully"

# Secondary check: Wait for pods to be ready
if kubectl wait --for=condition=Ready pod -l app.kubernetes.io/instance=my-redis -n helm-install-test --timeout=${TIMEOUT}s 2>/dev/null; then
    echo "✓ Pods are ready"
    exit 0
else
    echo "Warning: Helm release exists but pods not ready yet"
    kubectl get pods -n helm-install-test -o wide
    exit 1
fi
