# Getting Started with Kubernetes MCP Server

This guide walks you through the foundational setup for using the Kubernetes MCP Server with your Kubernetes cluster. You'll create a dedicated, read-only ServiceAccount and generate a secure kubeconfig file that can be used with various MCP clients.

> **Note:** This setup is **recommended for production use** but not strictly required. The MCP Server can use your existing kubeconfig file (e.g., `~/.kube/config`), but using a dedicated ServiceAccount with limited permissions follows the principle of least privilege and is more secure.

> **Next:** After completing this guide, continue with the [Claude Code CLI guide](GETTING_STARTED_CLAUDE_CODE.md). See the [docs README](README.md) for all available guides.

## What You'll Create

By the end of this guide, you'll have:
- A dedicated `mcp-viewer` ServiceAccount with read-only cluster access
- A secure, time-bound authentication token
- A dedicated kubeconfig file (`~/.kube/mcp-viewer.kubeconfig`)

## Prerequisites

- A running Kubernetes cluster
- `kubectl` CLI installed and configured
- Cluster admin permissions to create ServiceAccounts and RBAC bindings

## 1. Create a Read-Only ServiceAccount and RBAC

A ServiceAccount represents a non-human identity. Binding it to a read-only role lets tools query the cluster safely without using administrator credentials.

### Step 1.1: Create the Namespace and ServiceAccount

First, create a Namespace for the ServiceAccount:

```bash
# Create or pick a Namespace for the ServiceAccount
kubectl create namespace mcp

# Create the ServiceAccount
kubectl create serviceaccount mcp-viewer -n mcp
```

### Step 1.2: Grant Read-Only Access (RBAC)

Use a ClusterRoleBinding or RoleBinding to grant read-only permissions.

#### Option A: Cluster-Wide Read-Only (Most Common)

This binds the ServiceAccount to the built-in `view` ClusterRole, which provides read-only access across the whole cluster.

```bash
# Binds the view ClusterRole to the ServiceAccount
kubectl create clusterrolebinding mcp-viewer-crb \
    --clusterrole=view \
    --serviceaccount=mcp:mcp-viewer
```

#### Option B: Namespace-Scoped Only (Tighter Scope)

This limits read access to the `mcp` namespace only, using the built-in `view` Role.

```bash
# Binds the view role to the ServiceAccount within the 'mcp' namespace
kubectl create rolebinding mcp-viewer-rb \
    --role=view \
    --serviceaccount=mcp:mcp-viewer \
    -n mcp
```

### Quick Verification (Optional)

Verify the permissions granted to the ServiceAccount:

```bash
# Check if the ServiceAccount can list pods cluster-wide
# Expect 'yes' if you used the view ClusterRole (Option A)
kubectl auth can-i list pods --as=system:serviceaccount:mcp:mcp-viewer --all-namespaces
```

## 2. Mint a ServiceAccount Token

Tools authenticate via a bearer token. We use the TokenRequest API (`kubectl create token`) to generate a secure, short-lived token.

```bash
# Create a time-bound token (choose a duration, e.g., 2 hours)
TOKEN="$(kubectl create token mcp-viewer --duration=2h -n mcp)"

# Verify the token was generated (Optional)
echo "$TOKEN"
```

**Note:** The `kubectl create token` command requires Kubernetes v1.24+. For older versions, you'll need to extract the token from the ServiceAccount's secret.

## 3. Build a Dedicated Kubeconfig

A dedicated kubeconfig file isolates this ServiceAccount's credentials from your personal admin credentials, making it easy to point external tools at.

### Step 3.1: Get Cluster Details

Get the API server address and certificate authority from your current active context:

```bash
# 1. Get the current cluster API server address
API_SERVER="$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')"

# 2. Get the cluster's Certificate Authority (CA) file path or data
# First, try to get the CA file path
CA_FILE="$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.certificate-authority}')"

# If CA file is not set, extract the CA data and write it to a temp file
if [ -z "$CA_FILE" ]; then
    CA_FILE="/tmp/k8s-ca-$$.crt"
    kubectl config view --minify --raw -o jsonpath='{.clusters[0].cluster.certificate-authority-data}' | base64 -d > "$CA_FILE"
fi

# 3. Define the desired context name
CONTEXT_NAME="mcp-viewer-context"
KUBECONFIG_FILE="$HOME/.kube/mcp-viewer.kubeconfig"
```

### Step 3.2: Create and Configure the Kubeconfig File

Create the new kubeconfig file by defining the cluster, user (the ServiceAccount), and context:

```bash
# Create a new kubeconfig file with cluster configuration
kubectl config --kubeconfig="$KUBECONFIG_FILE" set-cluster mcp-viewer-cluster \
    --server="$API_SERVER" \
    --certificate-authority="$CA_FILE" \
    --embed-certs=true

# Set the ServiceAccount token as the user credential
kubectl config --kubeconfig="$KUBECONFIG_FILE" set-credentials mcp-viewer \
    --token="$TOKEN"

# Define the context (links the cluster and user)
kubectl config --kubeconfig="$KUBECONFIG_FILE" set-context "$CONTEXT_NAME" \
    --cluster=mcp-viewer-cluster \
    --user=mcp-viewer

# Set the new context as current
kubectl config --kubeconfig="$KUBECONFIG_FILE" use-context "$CONTEXT_NAME"

# Secure the file permissions
chmod 600 "$KUBECONFIG_FILE"

# Clean up temporary CA file if we created one
if [[ "$CA_FILE" == /tmp/k8s-ca-*.crt ]]; then
    rm -f "$CA_FILE"
fi
```

### Quick Sanity Check

You can now use this new file to verify access:

```bash
# Run a command using the dedicated kubeconfig file
kubectl --kubeconfig="$KUBECONFIG_FILE" get pods -A
```

This command should successfully list all Pods if you chose **Option A: Cluster-Wide Read-Only**, proving the ServiceAccount and its token are correctly configured.

## 4. Use with Kubernetes MCP Server

Now that you have a dedicated kubeconfig file, you can use it with the Kubernetes MCP Server:

```bash
# Run the MCP server with the dedicated kubeconfig
./kubernetes-mcp-server --kubeconfig="$HOME/.kube/mcp-viewer.kubeconfig"

# Or use npx
npx -y kubernetes-mcp-server@latest --kubeconfig="$HOME/.kube/mcp-viewer.kubeconfig"

# Or use uvx
uvx kubernetes-mcp-server@latest --kubeconfig="$HOME/.kube/mcp-viewer.kubeconfig"
```

Alternatively, you can set the `KUBECONFIG` environment variable:

```bash
export KUBECONFIG="$HOME/.kube/mcp-viewer.kubeconfig"
./kubernetes-mcp-server
```

## Token Expiration and Renewal

The token created in Step 2 has a limited lifetime (2 hours in the example). When it expires, you'll need to:

1. Generate a new token:
   ```bash
   TOKEN="$(kubectl create token mcp-viewer --duration=2h -n mcp)"
   ```

2. Update the kubeconfig file:
   ```bash
   kubectl config --kubeconfig="$KUBECONFIG_FILE" set-credentials mcp-viewer --token="$TOKEN"
   ```

For long-running applications, consider:
- Using a longer token duration (up to the cluster's maximum, typically 24h)
- Implementing automatic token renewal in your application
- Using a different authentication method (e.g., client certificates)

## Cleanup

To remove the ServiceAccount and associated RBAC bindings:

```bash
# Delete the ClusterRoleBinding (if using Option A)
kubectl delete clusterrolebinding mcp-viewer-crb

# Delete the RoleBinding (if using Option B)
kubectl delete rolebinding mcp-viewer-rb -n mcp

# Delete the ServiceAccount
kubectl delete serviceaccount mcp-viewer -n mcp

# Delete the namespace (optional - only if you created it specifically for this)
kubectl delete namespace mcp

# Remove the kubeconfig file
rm "$HOME/.kube/mcp-viewer.kubeconfig"
```

## Troubleshooting

### kubectl create token: command not found

This command requires Kubernetes v1.24+. For older versions, you'll need to extract the token from the ServiceAccount's secret manually.

### Permission denied errors

Ensure you're using the correct kubeconfig file and that the ServiceAccount has the necessary RBAC permissions. Verify with:

```bash
kubectl auth can-i list pods --as=system:serviceaccount:mcp:mcp-viewer --all-namespaces
```

## Next Steps

Now that you have a working kubeconfig with read-only access, configure Claude Code CLI:

- **[Using with Claude Code CLI](GETTING_STARTED_CLAUDE_CODE.md)** - Configure the MCP server with Claude Code CLI

You can also:
- Explore the [main README](../README.md) for more MCP server capabilities
