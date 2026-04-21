#!/usr/bin/env bash
NAMESPACE=simple-rbac-setup
SERVICE_ACCOUNT=pod-reader
SERVICE_ACCOUNT_USER="system:serviceaccount:${NAMESPACE}:${SERVICE_ACCOUNT}"

# Check for allowed permissions
if ! kubectl auth can-i list pods --as=${SERVICE_ACCOUNT_USER} -n=${NAMESPACE} &> /dev/null; then
    echo "ServiceAccount still can't list pods."
    exit 1
fi

# Check for denied permissions
if kubectl auth can-i list pods --as=${SERVICE_ACCOUNT_USER} -A &> /dev/null; then
    echo "ServiceAccount has excessive permissions (can 'list' pods in other namespaces)."
    exit 1
fi

echo "Verification successful: RBAC role correctly reconfigured."
exit 0
