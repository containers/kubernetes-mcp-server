#!/usr/bin/env bash
kubectl delete pod -n web test-connection --ignore-not-found
kubectl delete namespace web 
