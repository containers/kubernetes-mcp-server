#!/bin/bash
set -e

# Generate a placeholder self-signed CA certificate for KIND cluster startup
# This will be replaced with the real cert-manager CA after the cluster is created

CERT_DIR="_output/cert-manager-ca"
CA_CERT="$CERT_DIR/ca.crt"
CA_KEY="$CERT_DIR/ca.key"

mkdir -p "$CERT_DIR"

# Generate a self-signed CA certificate (valid placeholder)
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout "$CA_KEY" \
  -out "$CA_CERT" \
  -days 365 \
  -subj "/CN=placeholder-ca" \
  2>/dev/null

echo "✅ Placeholder CA certificate created at $CA_CERT"
echo "⚠️  This will be replaced with cert-manager CA after cluster creation"
