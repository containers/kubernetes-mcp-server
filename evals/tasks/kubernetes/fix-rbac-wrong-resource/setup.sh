#!/usr/bin/env bash
kubectl delete namespace simple-rbac-setup --ignore-not-found
kubectl create namespace simple-rbac-setup
kubectl create serviceaccount pod-reader -n simple-rbac-setup
# role is misconfigured to list deployments instead of pods
kubectl create role pod-reader-role --verb=list --resource=deployments -n simple-rbac-setup
kubectl create rolebinding pod-reader-binding --role=pod-reader-role --serviceaccount=simple-rbac-setup:pod-reader -n simple-rbac-setup
