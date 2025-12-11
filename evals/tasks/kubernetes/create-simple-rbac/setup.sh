#!/usr/bin/env bash
kubectl delete namespace create-simple-rbac --ignore-not-found # clean up, just in case
kubectl create namespace create-simple-rbac
kubectl create serviceaccount reader-sa -n create-simple-rbac
