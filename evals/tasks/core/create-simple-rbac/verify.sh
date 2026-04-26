#!/usr/bin/env bash

NAMESPACE="create-simple-rbac"
SERVICE_ACCOUNT="reader-sa"
SERVICE_ACCOUNT_USER="system:serviceaccount:${NAMESPACE}:${SERVICE_ACCOUNT}"

# Check for allowed permissions
if ! kubectl auth can-i get pods --as=$SERVICE_ACCOUNT_USER -n $NAMESPACE &> /dev/null; then
    echo "ServiceAccount cannot 'get' pods."
    exit 1
fi

if ! kubectl auth can-i list pods --as=$SERVICE_ACCOUNT_USER -n $NAMESPACE &> /dev/null; then
    echo "ServiceAccount cannot 'list' pods."
    exit 1
fi

# Check for denied permissions
if kubectl auth can-i delete pods --as=$SERVICE_ACCOUNT_USER -n $NAMESPACE &> /dev/null; then
  echo "ServiceAccount has excessive permissions (can 'delete' pods)."
  exit 1
fi

if kubectl auth can-i create pods --as=$SERVICE_ACCOUNT_USER -n $NAMESPACE &> /dev/null; then
  echo "ServiceAccount has excessive permissions (can 'create' pods)."
  exit 1
fi

if kubectl auth can-i create pods --as=$SERVICE_ACCOUNT_USER &> /dev/null; then
  echo "ServiceAccount has excessive permissions (can 'create' pods in other namespace)."
  exit 1
fi

if kubectl auth can-i list pods --as=$SERVICE_ACCOUNT_USER -A &> /dev/null; then
  echo "ServiceAccount has excessive permissions (can 'list' pods in other namespace)."
  exit 1
fi

echo "Verification successful: RBAC role and binding correctly configured."
exit 0
