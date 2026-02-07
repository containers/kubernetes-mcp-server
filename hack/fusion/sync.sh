#!/bin/bash
#
# IBM Fusion MCP Server - Upstream Sync Wrapper
#
# Convenient wrapper for the Python sync script
# Author: Sandeep Bazar (GitHub: sandeepbazar)
#

set -e

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PYTHON_SCRIPT="$SCRIPT_DIR/sync_upstream_and_apply_fusion_patch.py"

# Check if Python script exists
if [ ! -f "$PYTHON_SCRIPT" ]; then
    echo "Error: Python script not found at $PYTHON_SCRIPT"
    exit 1
fi

# Make sure Python script is executable
chmod +x "$PYTHON_SCRIPT"

# Run the Python script with all arguments
exec python3 "$PYTHON_SCRIPT" "$@"

# Made with Bob
