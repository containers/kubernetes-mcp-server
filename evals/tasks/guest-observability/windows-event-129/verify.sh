#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

python3 \
  "${SCRIPT_DIR}/../shared/verify-event129.py" \
  "event129"
