# Project Agents.md for Kubernetes MCP Server

This Agents.md file provides comprehensive guidance for AI assistants and coding agents (like Claude, Gemini, Cursor, and others) to work with this codebase.

This repository contains the kubernetes-mcp-server project,
a powerful Go-based Model Context Protocol (MCP) server that provides native Kubernetes and OpenShift cluster management capabilities without external dependencies.
This MCP server enables AI assistants (like Claude, Gemini, Cursor, and others) to interact with Kubernetes clusters using the Model Context Protocol (MCP).

## Project Structure and Repository layout

- Go package layout follows the standard Go conventions:
  - `cmd/kubernetes-mcp-server/` – main application entry point using Cobra CLI framework.
  - `pkg/` – libraries grouped by domain.
    - `api/` - API-related functionality, tool definitions, and toolset interfaces.
    - `config/` – configuration management.
    - `helm/` - Helm chart operations integration.
    - `http/` - HTTP server and authorization middleware.
    - `kubernetes/` - Kubernetes client management, authentication, and access control.
    - `mcp/` - Model Context Protocol (MCP) server implementation with tool registration and STDIO/HTTP support.
    - `output/` - output formatting and rendering.
    - `toolsets/` - Toolset registration and management for MCP tools.
    - `version/` - Version information management.
- `.github/` – GitHub-related configuration (Actions workflows, issue templates...).
- `docs/` – documentation files.
- `npm/` – Node packages that wraps the compiled binaries for distribution through npmjs.com.
- `python/` – Python package providing a script that downloads the correct platform binary from the GitHub releases page and runs it for distribution through pypi.org.
- `Dockerfile` - container image description file to distribute the server as a container image.
- `Makefile` – tasks for building, formatting, linting and testing.

## Feature development

Implement new functionality in the Go sources under `cmd/` and `pkg/`.
The JavaScript (`npm/`) and Python (`python/`) directories only wrap the compiled binary for distribution (npm and PyPI).
Most changes will not require touching them unless the version or packaging needs to be updated.

### Adding new MCP tools

The project uses a toolset-based architecture for organizing MCP tools:

- **Tool definitions** are created in `pkg/api/` using the `ServerTool` struct.
- **Toolsets** group related tools together (e.g., config tools, core Kubernetes tools, Helm tools).
- **Registration** happens in `pkg/toolsets/` where toolsets are registered at initialization.
- Each toolset lives in its own subdirectory under `pkg/toolsets/` (e.g., `pkg/toolsets/config/`, `pkg/toolsets/core/`, `pkg/toolsets/helm/`).

When adding a new tool:
1. Define the tool handler function that implements the tool's logic.
2. Create a `ServerTool` struct with the tool definition and handler.
3. Add the tool to an appropriate toolset (or create a new toolset if needed).
4. Register the toolset in `pkg/toolsets/` if it's a new toolset.

### Configuration Standards for Red Hat-Maintained Components

To ensure consistency across Red Hat-maintained toolsets and cluster providers, follow these standards when implementing certificate authority (CA) configuration:

#### Certificate Authority (CA) Configuration Standard

**Standard Pattern:** Certificate authorities must be configured using **file paths**, not inline PEM content.

**Implementation Guidelines:**

1. **Config Parser (`pkg/toolsets/<toolset>/config.go`):**
   - Accept `certificate_authority` as a string field in your toolset config struct.
   - In the toolset parser function, resolve relative paths using `config.ConfigDirPathFromContext(ctx)` to make them relative to the config file directory.
   - Store the resolved absolute path in the config struct.
   - Validate in the `Validate()` method that `certificate_authority` is a valid file path using `os.Stat()`.

   Example:
   ```go
   func (c *Config) Validate() error {
       // ... other validations ...
       
       // Validate that certificate_authority is a valid file
       if caValue := strings.TrimSpace(c.CertificateAuthority); caValue != "" {
           if _, err := os.Stat(caValue); err != nil {
               return fmt.Errorf("certificate_authority must be a valid file path: %w", err)
           }
       }
       return nil
   }
   
   func toolsetParser(ctx context.Context, primitive toml.Primitive, md toml.MetaData) (config.Extended, error) {
       var cfg Config
       if err := md.PrimitiveDecode(primitive, &cfg); err != nil {
           return nil, err
       }
       
       // Resolve relative CA file paths
       if cfg.CertificateAuthority != "" {
           configDir := config.ConfigDirPathFromContext(ctx)
           if configDir != "" && !filepath.IsAbs(cfg.CertificateAuthority) {
               cfg.CertificateAuthority = filepath.Join(configDir, cfg.CertificateAuthority)
           }
       }
       
       return &cfg, nil
   }
   ```

2. **Client Implementation:**
   - When using the CA certificate, read it from the file path using `os.ReadFile()`.
   - Only valid file paths are supported; the path must exist and be readable.

   Example:
   ```go
   func (c *Client) createHTTPClient() *http.Client {
       tlsConfig := &tls.Config{InsecureSkipVerify: c.insecure}
       
       if caValue := strings.TrimSpace(c.certificateAuthority); caValue != "" {
           // Read the certificate from file
           caPEM, err := os.ReadFile(caValue)
           if err != nil {
               // Handle error appropriately
           }
           
           // Use caPEM to configure TLS...
       }
       
       return &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConfig}}
   }
   ```

3. **Documentation:**
   - Document that `certificate_authority` accepts only a file path (absolute or relative).
   - Mention that relative paths are resolved relative to the config file directory.
   - Note that the file path must exist and be readable; invalid paths will cause validation to fail.

**Reference Implementation:**
- See `pkg/kiali/config.go` and `pkg/kiali/kiali.go` for a complete example.
- See `pkg/kubernetes-mcp-server/cmd/root.go` (lines 292-314) for how the main server handles CA files.

**Rationale:**
- Consistency with ACM cluster provider and other Red Hat-maintained components.
- Better security practices (certificates stored in files, not in config files).
- Easier management and rotation of certificates.
- Clearer separation of configuration and secrets.

## Building

Use the provided Makefile targets:

```bash
# Format source and build the binary
make build

# Build for all supported platforms
make build-all-platforms
```

`make build` will run `go fmt` and `go mod tidy` before compiling.
The resulting executable is `kubernetes-mcp-server`.

## Running

The README demonstrates running the server via
[`mcp-inspector`](https://modelcontextprotocol.io/docs/tools/inspector):

```bash
make build
npx @modelcontextprotocol/inspector@latest $(pwd)/kubernetes-mcp-server
```

To run the server locally, you can use `npx`, `uvx` or execute the binary directly:

```bash
# Using npx (Node.js package runner)
npx -y kubernetes-mcp-server@latest

# Using uvx (Python package runner)
uvx kubernetes-mcp-server@latest

# Binary execution
./kubernetes-mcp-server
```

This MCP server is designed to run both locally and remotely.

### Local Execution

When running locally, the server connects to a Kubernetes or OpenShift cluster using the kubeconfig file.
It reads the kubeconfig from the `--kubeconfig` flag, the `KUBECONFIG` environment variable, or defaults to `~/.kube/config`.

This means that `npx -y kubernetes-mcp-server@latest` on a workstation will talk to whatever cluster your current kubeconfig points to (e.g. a local Kind cluster).

### Remote Execution

When running remotely, the server can be deployed as a container image in a Kubernetes or OpenShift cluster.
The server can be run as a Deployment, StatefulSet, or any other Kubernetes resource that suits your needs.
The server will automatically use the in-cluster configuration to connect to the Kubernetes API server.

## Tests

Run all Go tests with:

```bash
make test
```

The test suite relies on the `setup-envtest` tooling from `sigs.k8s.io/controller-runtime`.
The first run downloads a Kubernetes `envtest` environment from the internet, so network access is required.
Without it some tests will fail during setup.

## Linting

Static analysis is performed with `golangci-lint`:

```bash
make lint
```

The `lint` target downloads the specified `golangci-lint` version if it is not already present under `_output/tools/bin/`.

## Additional Makefile targets

Beyond the basic build, test, and lint targets, the Makefile provides additional utilities:

**Local Development:**
```bash
# Setup a complete local development environment with Kind cluster
make local-env-setup

# Tear down the local Kind cluster
make local-env-teardown

# Show Keycloak status and connection info (for OIDC testing)
make keycloak-status

# Tail Keycloak logs
make keycloak-logs

# Install required development tools (like Kind) to ./_output/bin/
make tools
```

**Distribution and Publishing:**
```bash
# Copy compiled binaries to each npm package
make npm-copy-binaries

# Publish the npm packages
make npm-publish

# Publish the Python packages
make python-publish

# Update README.md with the latest toolsets
make update-readme-tools
```

Run `make help` to see all available targets with descriptions.

## Dependencies

When introducing new modules run `make tidy` so that `go.mod` and `go.sum` remain tidy.

## Coding style

- Go modules target Go **1.24** (see `go.mod`).
- Tests are written with the standard library `testing` package.
- Build, test and lint steps are defined in the Makefile—keep them working.

## Distribution Methods

The server is distributed as a binary executable, a Docker image, an npm package, and a Python package.

- **Native binaries** for Linux, macOS, and Windows are available in the GitHub releases.
- A **container image** (Docker) is built and pushed to the `quay.io/containers/kubernetes_mcp_server` repository.
- An **npm** package is available at [npmjs.com](https://www.npmjs.com/package/kubernetes-mcp-server).
  It wraps the platform-specific binary and provides a convenient way to run the server using `npx`.
- A **Python** package is available at [pypi.org](https://pypi.org/project/kubernetes-mcp-server/).
  It provides a script that downloads the correct platform binary from the GitHub releases page and runs it.
  It provides a convenient way to run the server using `uvx` or `python -m kubernetes_mcp_server`.
