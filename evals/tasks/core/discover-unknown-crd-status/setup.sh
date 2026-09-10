#!/usr/bin/env bash
set -euo pipefail

# Deleting the CRD (if it already exists from a prior run) cascades to any
# WidgetFleet instances too, so there's no need to delete those separately --
# and doing so would fail with "the server doesn't have a resource type" on
# a first run, before the CRD (and thus the "widgetfleet" resource type)
# exists at all.
kubectl delete crd widgetfleets.mcpeval.example.com --ignore-not-found

# A CRD the model cannot already know the apiVersion for from pretraining --
# the point of this task is to prove discovery, not recall.
cat <<EOF | kubectl apply -f -
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgetfleets.mcpeval.example.com
spec:
  group: mcpeval.example.com
  names:
    kind: WidgetFleet
    listKind: WidgetFleetList
    plural: widgetfleets
    singular: widgetfleet
  scope: Namespaced
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              x-kubernetes-preserve-unknown-fields: true
EOF

kubectl wait --for=condition=Established crd/widgetfleets.mcpeval.example.com --timeout=30s

# The sentinel value in spec.phase is checked for verbatim in the model's
# final answer (see verify: contains). It only appears here, so the model
# can only produce it by actually reading this object.
cat <<EOF | kubectl apply -f -
apiVersion: mcpeval.example.com/v1alpha1
kind: WidgetFleet
metadata:
  name: prod-fleet
  namespace: default
spec:
  phase: provisioning-quasar-77123
EOF
