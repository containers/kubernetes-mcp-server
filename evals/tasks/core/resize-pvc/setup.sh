#!/usr/bin/env bash
set -euo pipefail

# AWS provides expandable EBS classes; local clusters may use csi-hostpath-sc.
# Select an installed expandable class instead of assuming either provider.
storage_class=$(kubectl get storageclasses -o jsonpath='{range .items[?(@.allowVolumeExpansion==true)]}{.metadata.name}{"\n"}{end}' | awk 'NF {print; exit}')
if [[ -z "${storage_class}" ]]; then
    echo "ERROR: resize-pvc requires a StorageClass with allowVolumeExpansion=true" >&2
    exit 1
fi

kubectl delete namespace resize-pv --ignore-not-found
kubectl create namespace resize-pv

sed "s/__STORAGE_CLASS_NAME__/${storage_class}/" artifacts/storage-pvc.yaml | kubectl apply -f -
kubectl apply -f artifacts/storage-pod.yaml
