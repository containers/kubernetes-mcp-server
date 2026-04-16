# Microsoft Entra ID Setup for Kubernetes MCP Server

This guide shows you how to configure the Kubernetes MCP Server to use Microsoft Entra ID (formerly Azure AD) as the OIDC provider.

## Overview

Entra ID differs from Keycloak in that it only exposes the standard OpenID Connect discovery endpoint (`/.well-known/openid-configuration`) and does not implement the OAuth Authorization Server Metadata endpoints (`/.well-known/oauth-authorization-server`).

The MCP server automatically handles this by falling back to `openid-configuration` when the OAuth-specific endpoints return 404.

## Prerequisites

- Microsoft Entra ID admin access (Azure Portal)
- Kubernetes cluster configured with Entra ID as the OIDC provider
- `kubectl` CLI with cluster access

## Step 1: Register an App in Entra ID

### Create the App Registration

1. Go to **Azure Portal** → **Microsoft Entra ID** → **App registrations**
2. Click **New registration**
3. Fill in:
   - **Name:** `MCP Server` (or any name)
   - **Supported account types:** "Accounts in this organizational directory only"
   - **Redirect URI:** Leave blank for now
4. Click **Register**

### Note Your IDs

From the app's **Overview** page, copy:
- **Application (client) ID** → `CLIENT_ID`
- **Directory (tenant) ID** → `TENANT_ID`

### Configure Client Credentials

You need **one** of the following — a client secret or a certificate. If you only need MCP server authentication (no other systems sharing this app registration), certificate-based auth is recommended.

#### Option A: Client Secret

Use this if you prefer simplicity or if other systems (e.g., cluster console OIDC login) share this app registration and require a client secret.

1. Go to **Certificates & secrets** (left sidebar)
2. Click **New client secret**
3. Add description and expiration
4. Click **Add**
5. **Copy the Value immediately** (only shown once) → `CLIENT_SECRET`

#### Option B: Certificate (Recommended for MCP Server)

Use this for production deployments. No secret to manage — the MCP server authenticates using a signed JWT assertion.

1. Generate a certificate (or use your PKI):
   ```bash
   openssl req -x509 -newkey rsa:2048 -keyout client.key -out client.crt -days 365 -nodes -subj "/CN=MCP Server"
   ```
2. Go to **Certificates & secrets** (left sidebar)
3. Click the **Certificates** tab
4. Click **Upload certificate** and select `client.crt`
5. Note the **Thumbprint** shown after upload — you can use this to verify your config later

> **Tip:** If your cluster already uses a separate app registration with a client secret for console OIDC login, you can create a dedicated app registration for the MCP server using certificate auth only (no secret needed). See [Separate App Registration for MCP Server](#separate-app-registration-for-mcp-server) below.

### Configure API Permissions

1. Go to **API permissions** (left sidebar)
2. Click **Add a permission** → **Microsoft Graph** → **Delegated permissions**
3. Add these permissions:
   - `openid`
   - `profile`
   - `email`
4. Click **Add permissions**
5. Click **Grant admin consent for [your org]**

### Configure Token Claims

1. Go to **Token configuration** (left sidebar)
2. Click **Add optional claim**
3. Select **ID** token type
4. Check these claims:
   - `email`
   - `preferred_username`
5. Click **Add**

### Add Redirect URI (Optional - for Testing)

If you plan to test with MCP Inspector:

1. Go to **Authentication** (left sidebar)
2. Under **Platform configurations**, click **Add a platform** → **Web**
3. Add redirect URI: `http://localhost:6274/oauth/callback`
4. Click **Configure**

## Step 2: Configure MCP Server

Create a configuration file (`config.toml`):

### Basic Configuration

Use this configuration when your Kubernetes cluster accepts Entra ID tokens directly (cluster OIDC is configured with the same Entra ID tenant):

```toml
require_oauth = true
oauth_audience = "<CLIENT_ID>"
oauth_scopes = ["openid", "profile", "email"]

# Entra ID uses v2.0 endpoints
authorization_url = "https://login.microsoftonline.com/<TENANT_ID>/v2.0"
```

Replace:
- `<CLIENT_ID>` with your Application (client) ID
- `<TENANT_ID>` with your Directory (tenant) ID

> **Note:** When `cluster_auth_mode` is not set, the server auto-detects:
> - If `require_oauth = true` → uses `passthrough`
> - Otherwise → uses `kubeconfig`
>
> In `passthrough` mode, if token exchange is configured (`token_exchange_strategy` or `sts_audience`), the token is exchanged before being passed to the cluster.

### With ServiceAccount Credentials

If your Kubernetes cluster doesn't accept Entra ID tokens on the API server, use this configuration:

```toml
require_oauth = true
oauth_audience = "<CLIENT_ID>"
oauth_scopes = ["openid", "profile", "email"]

authorization_url = "https://login.microsoftonline.com/<TENANT_ID>/v2.0"

# Use kubeconfig ServiceAccount credentials for cluster access
cluster_auth_mode = "kubeconfig"
kubeconfig = "/path/to/sa-kubeconfig"
```

This setup:
- **MCP clients authenticate via Entra ID** (OAuth required for MCP access)
- **Cluster access uses ServiceAccount token** (from kubeconfig)

#### Creating a ServiceAccount Kubeconfig

Your regular kubeconfig likely uses interactive login. Create a kubeconfig with a static ServiceAccount token:

```bash
# Create ServiceAccount
kubectl create sa mcp-server -n default

# Grant permissions (adjust role as needed)
kubectl create clusterrolebinding mcp-server-reader \
  --clusterrole=view \
  --serviceaccount=default:mcp-server

# Create a token (adjust duration to your security requirements)
kubectl create token mcp-server -n default --duration=720h > sa-token

# Create kubeconfig with the token
export SA_TOKEN=$(cat sa-token)
export CLUSTER_URL=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')
export CLUSTER_CA=$(kubectl config view --raw --minify -o jsonpath='{.clusters[0].cluster.certificate-authority-data}')

cat > mcp-kubeconfig << EOF
apiVersion: v1
kind: Config
clusters:
- cluster:
    certificate-authority-data: ${CLUSTER_CA}
    server: ${CLUSTER_URL}
  name: cluster
contexts:
- context:
    cluster: cluster
    user: mcp-server
  name: mcp-context
current-context: mcp-context
users:
- name: mcp-server
  user:
    token: ${SA_TOKEN}
EOF
```

Then run:
```bash
./kubernetes-mcp-server --config config.toml
```

### With Token Exchange (On-Behalf-Of Flow)

If your cluster accepts Entra ID tokens and you want to exchange the user's token via the On-Behalf-Of (OBO) flow, use one of the credential options below.

#### With Client Secret (Option A)

```toml
require_oauth = true
oauth_audience = "<CLIENT_ID>"
oauth_scopes = ["openid", "profile", "email"]

authorization_url = "https://login.microsoftonline.com/<TENANT_ID>/v2.0"

# Token exchange configuration (passthrough will use this automatically)
token_exchange_strategy = "entra-obo"
sts_client_id = "<CLIENT_ID>"
sts_client_secret = "<CLIENT_SECRET>"
sts_scopes = ["api://<DOWNSTREAM_API_APP_ID>/.default"]
```

#### With Certificate (Option B — Recommended)

```toml
require_oauth = true
oauth_audience = "<CLIENT_ID>"
oauth_scopes = ["openid", "profile", "email"]

authorization_url = "https://login.microsoftonline.com/<TENANT_ID>/v2.0"

# Token exchange with certificate authentication (RFC 7523 JWT Client Assertion)
token_exchange_strategy = "entra-obo"
sts_client_id = "<CLIENT_ID>"
sts_auth_style = "assertion"
sts_client_cert_file = "/path/to/client.crt"
sts_client_key_file = "/path/to/client.key"
sts_scopes = ["api://<DOWNSTREAM_API_APP_ID>/.default"]
```

No client secret is needed when using certificate auth. The MCP server signs a short-lived JWT assertion (5 minutes) using the private key, and Entra ID validates it against the uploaded certificate.

#### OBO Prerequisites

For OBO to work, you need to configure API permissions in Azure:
1. Go to your app registration → **API permissions**
2. Click **Add a permission** → **APIs my organization uses**
3. Select the downstream API app registration
4. Add the required delegated permissions

## Step 3: Run the MCP Server

```bash
./kubernetes-mcp-server --config config.toml
```

## Testing with MCP Inspector (Optional)

To test authentication with MCP Inspector:

1. Ensure redirect URI is configured (see Step 1)
2. Start MCP Inspector:
   ```bash
   npx @modelcontextprotocol/inspector@latest $(pwd)/kubernetes-mcp-server --config config.toml
   ```
3. In **Authentication** section:
   - Set **Client ID** to your `<CLIENT_ID>`
   - Set **Scope** to `openid profile email`
4. Click **Connect**
5. Login with your Entra ID credentials

## How It Works

### Client Registration

Entra ID doesn't support RFC 7591 Dynamic Client Registration - clients must be pre-registered in the Azure portal (as shown in Step 1 above).

Add redirect URIs in the Azure portal → Authentication for your MCP clients:
- `http://localhost:6274/oauth/callback` (MCP Inspector default)

### Well-Known Endpoint Fallback

The MCP server implements automatic fallback for OIDC providers that don't support all OAuth 2.0 well-known endpoints:

1. When a client requests `/.well-known/oauth-authorization-server`, the server first tries to proxy the request to Entra ID
2. Entra ID returns 404 (this endpoint doesn't exist)
3. The server automatically falls back to fetching `/.well-known/openid-configuration`
4. The openid-configuration response is returned, which contains all required OAuth metadata

This allows MCP clients to work with Entra ID without any special configuration.

## Troubleshooting

### "invalid_client" Error

Check that:
- You're using the correct client ID
- The redirect URI matches exactly what's configured in Entra ID
- The client secret is correct (if using client secret auth)

### "AADSTS700027" Certificate Not Registered

This means the certificate used to sign the JWT assertion doesn't match any certificate uploaded to your app registration.

1. Check your certificate's thumbprint:
   ```bash
   openssl x509 -in /path/to/client.crt -fingerprint -sha1 -noout
   ```
2. Go to Azure Portal → App registrations → your app → **Certificates & secrets** → **Certificates**
3. Compare the thumbprint. If it doesn't match, upload the correct certificate
4. Make sure `sts_client_cert_file` and `sts_client_key_file` point to the matching cert/key pair

### "AADSTS50011" Redirect URI Mismatch

The redirect URI in your request doesn't match Entra ID configuration:
1. Go to Azure Portal → App registrations → your app → Authentication
2. Add the exact redirect URI shown in the error message

### Token Validation Fails

Ensure your Kubernetes cluster is configured to trust Entra ID tokens:
- The OIDC issuer should be `https://login.microsoftonline.com/{tenant}/v2.0`
- The audience should match your client ID or application ID URI

### Well-Known Endpoint Returns 404

This is expected for `oauth-authorization-server` and `oauth-protected-resource` endpoints. The MCP server automatically handles this by falling back to `openid-configuration`.

## Differences from Keycloak

| Feature | Keycloak | Entra ID |
|---------|----------|----------|
| oauth-authorization-server endpoint | ✅ Supported | ❌ Not available |
| oauth-protected-resource endpoint | ✅ Supported | ❌ Not available |
| openid-configuration endpoint | ✅ Supported | ✅ Supported |
| Token Exchange (RFC 8693) | ✅ Supported | ❌ Use On-Behalf-Of flow |
| Dynamic Client Registration | ✅ Supported | ❌ Not available |

The MCP server handles these differences automatically through the well-known endpoint fallback mechanism.

## Quick Reference

| Item | Where to Find |
|------|---------------|
| Client ID | Azure Portal → App registrations → Overview → Application (client) ID |
| Tenant ID | Azure Portal → App registrations → Overview → Directory (tenant) ID |
| Client Secret | Azure Portal → App registrations → Certificates & secrets → Value column |
| Authorization URL | `https://login.microsoftonline.com/<TENANT_ID>/v2.0` |

## Configuring Your Cluster to Accept Entra ID Tokens

For the passthrough flow to work, your Kubernetes cluster's API server must be configured to accept Entra ID tokens via OIDC. This is separate from any console or dashboard login configuration your cluster may have.

### Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                                                                             │
│  ┌──────────┐     ┌─────────────┐     ┌──────────┐     ┌─────────────────┐ │
│  │   User   │────▶│ MCP Client  │────▶│MCP Server│────▶│  Kubernetes     │ │
│  │          │     │ (Inspector) │     │          │     │  Cluster        │ │
│  └──────────┘     └─────────────┘     └──────────┘     └─────────────────┘ │
│       │                 │                   │                   │          │
│       │   1. OAuth      │   2. Bearer       │   3. OBO +        │          │
│       │      Login      │      Token        │      Assertion    │          │
│       │                 │                   │                   │          │
│       ▼                 ▼                   ▼                   ▼          │
│  ┌──────────────────────────────────────────────────────────────────────┐  │
│  │                        Microsoft Entra ID                            │  │
│  │                                                                      │  │
│  │  1. User authenticates via OAuth 2.0 (authorization code flow)      │  │
│  │  2. MCP Server validates user token                                 │  │
│  │  3. MCP Server exchanges token using OBO + JWT client assertion     │  │
│  │  4. Cluster validates exchanged token via OIDC                      │  │
│  └──────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Prerequisites

- Cluster admin access
- Ability to configure kube-apiserver OIDC flags (managed clusters may expose this differently)

### Configure kube-apiserver OIDC Flags

The API server needs the following OIDC flags. How you set these depends on your cluster type:

- **kubeadm / self-managed**: edit `/etc/kubernetes/manifests/kube-apiserver.yaml`
- **Managed Kubernetes (EKS, AKS, GKE)**: use the provider's OIDC identity configuration
- **Kind / Minikube**: pass flags via cluster config

The required flags:

```
--oidc-issuer-url=https://login.microsoftonline.com/<TENANT_ID>/v2.0
--oidc-client-id=<CLIENT_ID>
--oidc-username-claim=preferred_username
--oidc-groups-claim=groups
```

### Create RBAC for Entra ID Users

Once the API server accepts Entra ID tokens, create RBAC bindings for your users:

```bash
# For a specific user
kubectl create clusterrolebinding entra-user-admin \
  --clusterrole=cluster-admin \
  --user="user@yourdomain.com"

# For a group (requires groups claim configured in Entra ID)
kubectl create clusterrolebinding entra-group-admin \
  --clusterrole=cluster-admin \
  --group="your-group-object-id"
```

### Verify OIDC Is Working

You can test that the API server accepts Entra ID tokens by using `kubectl` with a token:

```bash
kubectl --token="<ENTRA_ID_ACCESS_TOKEN>" get namespaces
```

If this returns namespaces, your cluster is correctly configured.

### Complete MCP Server Config for Passthrough

```toml
# config.toml
log_level = 4
port = "8080"

# OAuth: Users authenticate via Entra ID
require_oauth = true
authorization_url = "https://login.microsoftonline.com/<TENANT_ID>/v2.0"
oauth_audience = "<CLIENT_ID>"

# Pass exchanged token to cluster
cluster_auth_mode = "passthrough"

# Token Exchange: OBO flow with JWT client assertion
token_exchange_strategy = "entra-obo"
sts_client_id = "<CLIENT_ID>"
sts_auth_style = "assertion"
sts_client_cert_file = "/path/to/client.crt"
sts_client_key_file = "/path/to/client.key"
sts_scopes = ["<CLIENT_ID>/.default"]
```

### Understanding the Two Trust Relationships

1. **MCP Server → Entra ID (OBO Exchange)**
   - MCP Server authenticates using JWT client assertion (certificate)
   - No client_secret needed
   - This is what `sts_auth_style = "assertion"` configures

2. **Cluster → Entra ID (Token Validation)**
   - The API server validates tokens from Entra ID
   - Configured via kube-apiserver OIDC flags
   - Uses OIDC discovery to fetch signing keys

Both relationships use the same app registration but serve different purposes.

## Separate App Registration for MCP Server

If your cluster already uses an Entra ID app registration with a client secret for console OIDC login, you may want a **separate app registration** for the MCP server — especially if you prefer certificate-based auth and don't want to add a certificate to the existing app.

### When to Use This

- Your cluster's OIDC app registration is shared with other systems (console, CLI) and uses a client secret
- You want the MCP server to use certificate auth without affecting the existing setup
- You want to scope the MCP server's permissions separately

### Setup

**App Registration A** (existing) — used by the cluster for console/CLI OIDC login. Has a client secret. No changes needed.

**App Registration B** (new) — used by the MCP server for OBO token exchange. Uses certificate auth, no secret required.

1. Create a new app registration in Azure (follow [Step 1](#step-1-register-an-app-in-entra-id) above)
2. Choose **Option B: Certificate** for credentials — skip the client secret
3. On App Registration B, go to **Expose an API** (left sidebar):
   - Click **Set** next to "Application ID URI" (accept the default `api://<CLIENT_ID_B>`)
   - Click **Add a scope** → name it (e.g., `access_as_user`) → set "Who can consent" to "Admins and users" → enable it
4. On App Registration A (the existing one), go to **API permissions**:
   - Click **Add a permission** → **APIs my organization uses** → find App Registration B
   - Add the delegated scope you created (e.g., `access_as_user`)
   - Click **Grant admin consent**

### MCP Server Configuration

```toml
require_oauth = true
# Use App A's client ID — this is what MCP clients authenticate with
oauth_audience = "<CLIENT_ID_A>"
oauth_scopes = ["openid", "profile", "email"]

authorization_url = "https://login.microsoftonline.com/<TENANT_ID>/v2.0"

# OBO exchange uses App B's credentials (certificate, no secret)
token_exchange_strategy = "entra-obo"
sts_client_id = "<CLIENT_ID_B>"
sts_auth_style = "assertion"
sts_client_cert_file = "/path/to/client.crt"
sts_client_key_file = "/path/to/client.key"
sts_scopes = ["<CLIENT_ID_B>/.default"]
```

This way, the cluster's existing OIDC configuration is untouched, and the MCP server has its own credentials with certificate-based auth.

## See Also

- [Entra ID OAuth 2.0 Documentation](https://learn.microsoft.com/en-us/entra/identity-platform/v2-oauth2-auth-code-flow)
- [Entra ID On-Behalf-Of Flow](https://learn.microsoft.com/en-us/entra/identity-platform/v2-oauth2-on-behalf-of-flow)
- [Kubernetes OIDC Authentication](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#openid-connect-tokens)
- [Keycloak OIDC Setup](KEYCLOAK_OIDC_SETUP.md) - Alternative OIDC provider setup
