#!/usr/bin/env bash
set -e

# Keycloak Certificate Generation Script
# Generates self-signed CA and Keycloak server certificate for local development

CERTS_DIR="hack/keycloak-certs"
KEYCLOAK_HOSTNAME="keycloak.127-0-0-1.sslip.io"

echo "========================================="
echo "Generating Keycloak TLS Certificates"
echo "========================================="
echo ""

# Create certificates directory
mkdir -p "$CERTS_DIR"

# Generate CA private key
echo "Generating CA private key..."
openssl genrsa -out "$CERTS_DIR/ca.key" 4096

# Generate CA certificate
echo "Generating CA certificate..."
openssl req -x509 -new -nodes \
  -key "$CERTS_DIR/ca.key" \
  -sha256 -days 3650 \
  -out "$CERTS_DIR/ca.crt" \
  -subj "/C=US/ST=State/L=City/O=Kubernetes MCP Server/CN=Keycloak CA"

echo "✅ CA certificate generated"
echo ""

# Generate Keycloak server private key
echo "Generating Keycloak server private key..."
openssl genrsa -out "$CERTS_DIR/keycloak.key" 4096

# Generate Keycloak server CSR
echo "Generating Keycloak server certificate signing request..."
openssl req -new \
  -key "$CERTS_DIR/keycloak.key" \
  -out "$CERTS_DIR/keycloak.csr" \
  -subj "/C=US/ST=State/L=City/O=Kubernetes MCP Server/CN=${KEYCLOAK_HOSTNAME}"

# Create SAN configuration file
cat > "$CERTS_DIR/san.cnf" <<EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
prompt = no

[req_distinguished_name]
C = US
ST = State
L = City
O = Kubernetes MCP Server
CN = ${KEYCLOAK_HOSTNAME}

[v3_req]
keyUsage = keyEncipherment, dataEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = ${KEYCLOAK_HOSTNAME}
DNS.2 = keycloak
DNS.3 = keycloak.keycloak
DNS.4 = keycloak.keycloak.svc
DNS.5 = keycloak.keycloak.svc.cluster.local
DNS.6 = localhost
IP.1 = 127.0.0.1
EOF

# Sign the certificate with CA
echo "Signing Keycloak server certificate with CA..."
openssl x509 -req \
  -in "$CERTS_DIR/keycloak.csr" \
  -CA "$CERTS_DIR/ca.crt" \
  -CAkey "$CERTS_DIR/ca.key" \
  -CAcreateserial \
  -out "$CERTS_DIR/keycloak.crt" \
  -days 365 \
  -sha256 \
  -extfile "$CERTS_DIR/san.cnf" \
  -extensions v3_req

echo "✅ Keycloak server certificate generated"
echo ""

# Clean up temporary files
rm -f "$CERTS_DIR/keycloak.csr" "$CERTS_DIR/san.cnf" "$CERTS_DIR/ca.srl"

echo "========================================="
echo "Certificate Generation Complete"
echo "========================================="
echo ""
echo "Generated files:"
echo "  CA Certificate:     $CERTS_DIR/ca.crt"
echo "  CA Key:             $CERTS_DIR/ca.key"
echo "  Server Certificate: $CERTS_DIR/keycloak.crt"
echo "  Server Key:         $CERTS_DIR/keycloak.key"
echo ""
echo "Certificate valid for: ${KEYCLOAK_HOSTNAME}"
echo "========================================="
