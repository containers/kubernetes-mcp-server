# Keycloak OIDC Setup for Kubernetes MCP Server

This guide shows you how to set up a local development environment with Keycloak for OIDC authentication testing.

## Overview

The local development environment includes:
- **Kind cluster** with OIDC-enabled API server
- **Keycloak** (deployed in the cluster) for OIDC provider
- **Kubernetes MCP Server** configured for OAuth/OIDC authentication

## Quick Start

Set up the complete environment with one command:

```bash
make local-env-setup
```

This will:
1. Install required tools (kind) to `./_output/bin/`
2. Create a Kind cluster with OIDC configuration
3. Deploy Keycloak in the cluster
4. Configure Keycloak realm and clients
5. Build the MCP server binary
6. Generate a configuration file at `_output/config.toml`

## Running the MCP Server

After setup completes, run the server:

```bash
# Start the server
./kubernetes-mcp-server --port 8008 --config _output/config.toml
```

Or use the MCP Inspector for testing:

```bash
npx @modelcontextprotocol/inspector@latest $(pwd)/kubernetes-mcp-server --config _output/config.toml
```

## Quick Walkthrough

### 1. Start MCP Inspector and Connect

After running the inspector, in the `Authentication`'s **OAuth 2.0 Flow** set the `Client ID` to be `mcp-client` and the `Scope` to `mcp-server`, afterwards click the "Connect" button.

<a href="images/keycloak-mcp-inspector-connect.png">
  <img src="images/keycloak-mcp-inspector-connect.png" alt="MCP Inspector Connect Button" width="600" />
</a>

### 2. Login to Keycloak

You'll be redirected to Keycloak. Enter the test credentials:
- Username: `mcp`
- Password: `mcp`

<a href="images/keycloak-login-page.png">
  <img src="images/keycloak-login-page.png" alt="Keycloak Login Page" width="600" />
</a>

### 3. Use MCP Tools

After authentication, you can use the **Tools** from the Kubernetes-MCP-Server from the MCP Inspector, like below where we run the `pods_list` tool, to list all pods in the current cluster from all namespaces.

<a href="images/keycloak-mcp-inspector-results.png">
  <img src="images/keycloak-mcp-inspector-results.png" alt="MCP Inspector Tool Results" width="600" />
</a>

## Architecture

### Keycloak Deployment
- Runs as a StatefulSet in the `keycloak` namespace
- Exposed via Ingress with TLS at `https://keycloak.127-0-0-1.sslip.io:8443`
- Uses cert-manager for TLS certificates
- Accessible from both host and cluster pods

### Kind Cluster with OIDC
- Kubernetes API server configured with OIDC authentication
- Points to Keycloak's `openshift` realm as the OIDC issuer
- Validates bearer tokens against Keycloak's JWKS endpoint
- API server trusts the cert-manager CA certificate

### Authentication Flow

```
User Browser
    |
    | 1. OAuth login (https://keycloak.127-0-0-1.sslip.io:8443)
    v
Keycloak
    |
    | 2. ID Token (aud: mcp-server)
    v
MCP Server
    |
    | 3. Token Exchange (aud: openshift)
    v
Keycloak
    |
    | 4. Exchanged Access Token
    v
MCP Server
    |
    | 5. Bearer Token in API request
    v
Kubernetes API Server
    |
    | 6. Validate token via OIDC
    v
Keycloak JWKS
    |
    | 7. Token valid, execute tool
    v
MCP Server → User
```

## Keycloak Configuration

The setup automatically configures:

### Realm: `openshift`
- Token lifespan: 30 minutes
- Session idle timeout: 30 minutes

### Clients

1. **mcp-client** (public)
   - Public client for browser-based OAuth login
   - PKCE required for security
   - Valid redirect URIs: `*`

2. **mcp-server** (confidential)
   - Confidential client with client secret
   - Standard token exchange enabled
   - Can exchange tokens with `aud: openshift`
   - Default scopes: `openid`, `groups`, `mcp-server`
   - Optional scopes: `mcp:openshift`

3. **openshift** (confidential)
   - Target client for token exchange
   - Accepts exchanged tokens from `mcp-server`
   - Used by Kubernetes API server for OIDC validation

### Client Scopes
- **mcp-server**: Default scope with audience mapper
- **mcp:openshift**: Optional scope for token exchange with audience mapper
- **groups**: Group membership mapper (included in tokens)

### Default User
- **Username**: `mcp`
- **Password**: `mcp`
- **Email**: `mcp@example.com`
- **RBAC**: `cluster-admin` (full cluster access)

## MCP Server Configuration

The generated `_output/config.toml` includes:

```toml
require_oauth = true
oauth_audience = "mcp-server"
oauth_scopes = ["openid", "mcp-server"]
validate_token = false  # Validation done by K8s API server
authorization_url = "https://keycloak.127-0-0-1.sslip.io:8443/realms/openshift"

sts_client_id = "mcp-server"
sts_client_secret = "..."  # Auto-generated
sts_audience = "openshift"  # Triggers token exchange
sts_scopes = ["mcp:openshift"]

certificate_authority = "_output/cert-manager-ca/ca.crt"  # For HTTPS validation
```

## Useful Commands

### Check Keycloak Status

```bash
make keycloak-status
```

Shows:
- Keycloak pod status
- Service endpoints
- Access URL
- Admin credentials

### View Keycloak Logs

```bash
make keycloak-logs
```

### Access Keycloak Admin Console

Open your browser to:
```
https://keycloak.127-0-0-1.sslip.io:8443
```

**Admin credentials:**
- Username: `admin`
- Password: `admin`

Navigate to the `openshift` realm to view/modify the configuration.

## Teardown

Remove the local environment:

```bash
make local-env-teardown
```

This deletes the Kind cluster (Keycloak is removed with it).
