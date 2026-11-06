#!/usr/bin/env bash
# Shared verify helper: fails unless $AGENT_OUTPUT contains the given string (case-insensitive, literal).

require() {
  if ! grep -qiF -- "$1" <<<"$AGENT_OUTPUT"; then
    echo "response is missing required evidence: $1" >&2
    exit 1
  fi
}
