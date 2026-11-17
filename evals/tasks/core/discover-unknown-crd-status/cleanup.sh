#!/usr/bin/env bash
kubectl delete widgetfleet prod-fleet -n default --ignore-not-found
kubectl delete crd widgetfleets.mcpeval.example.com --ignore-not-found
