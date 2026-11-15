#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

GUEST_OBSERVABILITY_SCENARIO=event129-missing-vm-name   "${SCRIPT_DIR}/../shared/setup-mock.sh"
