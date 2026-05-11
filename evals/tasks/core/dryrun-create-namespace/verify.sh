#!/usr/bin/env bash
# Verify namespace was not created (dry run).
TIMEOUT="10s"

# Wait for namespace to be created
if kubectl wait --for=create --timeout=$TIMEOUT namespace/dryrun-create-namespace; then
    echo "Namespace was created, but it should have been a dry run"
    exit 1
fi

# Namespace was not created due to dry run flag - success
exit 0
