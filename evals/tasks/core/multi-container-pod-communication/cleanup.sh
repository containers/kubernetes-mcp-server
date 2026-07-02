#!/usr/bin/env bash

kubectl delete pod communication-pod -n multi-container-logging --ignore-not-found
kubectl delete configmap shared-data -n multi-container-logging --ignore-not-found
kubectl delete namespace multi-container-logging --ignore-not-found
