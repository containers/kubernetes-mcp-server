#!/usr/bin/env bash

kubectl delete namespace multi-container-logging --ignore-not-found
kubectl create namespace multi-container-logging
