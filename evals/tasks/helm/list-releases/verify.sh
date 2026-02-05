#!/usr/bin/env bash
# Verify that helm list was called and returns the test release
# The agent should have discovered the test-nginx release

if helm list -n helm-list-test | grep -q "test-nginx"; then
    exit 0
else
    echo "Verification failed: test-nginx release not found in helm list output"
    exit 1
fi
