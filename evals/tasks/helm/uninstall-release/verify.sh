#!/usr/bin/env bash
# Verify that the helm release was uninstalled
# The release should no longer appear in helm list

if helm list -n helm-uninstall-test | grep -q "cleanup-test"; then
    echo "Verification failed: cleanup-test release still exists"
    exit 1
else
    # Success - the release is gone
    exit 0
fi
