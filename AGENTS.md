# Project Agents.md for Kubernetes MCP Server

This Agents.md file provides comprehensive guidance for AI assistants and coding agents (like Claude, Gemini, Cursor, and others) to work with this codebase.

This repository contains the kubernetes-mcp-server project,
a powerful Go-based Model Context Protocol (MCP) server that provides native Kubernetes and OpenShift cluster management capabilities without external dependencies.
This MCP server enables AI assistants (like Claude, Gemini, Cursor, and others) to interact with Kubernetes clusters using the Model Context Protocol (MCP).

## Project Structure

Standard Go layout: `cmd/kubernetes-mcp-server/` (entry point), `pkg/` (libraries by domain).
The `npm/` and `python/` directories only wrap the compiled binary for distribution — do not add features there.
Build, test, and lint commands are in the `Makefile`.

## Feature development

### Logging

When adding log lines, always use a contextual logger (`klogutil.FromContext(ctx)` from `pkg/klogutil`). If necessary, add a `ctx` parameter to the function, and wire
the context through to where you need a logger.

`klogutil.FromContext` wraps `klog.FromContext` and injects the context into the logger's values so
the OpenTelemetry log bridge can extract the active trace span for log-trace correlation. Do not use
`klog.FromContext` directly in production code.

If you start a new trace span (e.g. `ctx, span := tracer.Start(ctx, "op")`), you must call
`klogutil.FromContext(ctx)` again to pick up the new span — the logger captured before the span was
created still carries the old (or empty) span context.

### Adding new MCP tools

The project uses a toolset-based architecture for organizing MCP tools:

- **Tool definitions** are created in `pkg/api/` using the `ServerTool` struct.
- **Toolsets** group related tools together (e.g., config tools, core Kubernetes tools, Helm tools).
- **Registration** happens in `pkg/toolsets/` where toolsets are registered at initialization.
- Each toolset lives in its own subdirectory under `pkg/toolsets/` (e.g., `pkg/toolsets/config/`, `pkg/toolsets/core/`, `pkg/toolsets/helm/`).

**Important:** When creating a new toolset, adding tools to an existing toolset, or modifying tool definitions, **always use the `/toolset-design` skill first**. This skill validates the design (naming, grouping, input schema, eval coverage) before implementation begins.

When adding a new tool:
1. Define the tool handler function that implements the tool's logic.
2. Create a `ServerTool` struct with the tool definition and handler.
3. Add the tool to an appropriate toolset (or create a new toolset if needed).
4. Register the toolset in `pkg/toolsets/` if it's a new toolset.

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

This means that `npx -y kubernetes-mcp-server@latest` on a workstation will talk to whatever cluster your current kubeconfig points to (e.g. a local Minikube cluster).

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

Before writing or modifying tests, read [`docs/dev/testing.md`](docs/dev/testing.md) for the project's testing patterns, downstream compatibility rules, and examples.

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
# Setup a complete local development environment with Minikube cluster
make local-env-setup

# Tear down the local Minikube cluster
make local-env-teardown

# Show Keycloak status and connection info (for OIDC testing)
make keycloak-status

# Tail Keycloak logs
make keycloak-logs
```

**Distribution and Publishing:**
```bash
# Copy compiled binaries to each npm package
make npm-copy-binaries

# Publish the npm packages
make npm-publish

# Publish the Python packages
make python-publish

# Update README.md and docs/configuration.md with the latest toolsets
make update-readme-tools
```

Run `make help` to see all available targets with descriptions.

## Dependencies

When introducing new modules run `make tidy` so that `go.mod` and `go.sum` remain tidy.

## Coding style

- The Go version is declared in `go.mod`; CI installs whatever it requires via `go-version-file`.
- Tests are written with the standard library `testing` package.
- Build, test and lint steps are defined in the Makefile—keep them working.

## Documentation

The `docs/` directory contains user-facing documentation:

- `docs/README.md` – Documentation index and navigation
- `docs/configuration.md` – **Complete TOML configuration reference** (all `StaticConfig` options, drop-in configuration, dynamic reload)
- `docs/prompts.md` – MCP Prompts configuration guide
- `docs/logging.md` – MCP Logging guide (automatic K8s error logging, secret redaction)
- `docs/OTEL.md` – OpenTelemetry observability setup
- `docs/KIALI.md` – Kiali toolset configuration
- `docs/getting-started-kubernetes.md` – Kubernetes ServiceAccount setup
- `docs/getting-started-claude-code.md` – Claude Code CLI integration
- `docs/KEYCLOAK_OIDC_SETUP.md` – OAuth/OIDC developer setup

The `docs/specs/` directory contains feature specifications (living documentation for coding agents):

- `docs/specs/validation.md` – Pre-execution validation layer specification (resource existence, schema, RBAC)

### Documentation conventions

- Use **lowercase filenames** for new documentation files (e.g., `configuration.md`, `prompts.md`)
- The toolsets table, tools, prompts, resources, and resource templates in `README.md` and `docs/configuration.md` are **auto-generated** - use `make update-readme-tools` to update them after modifying toolsets
- Both files use marker pairs for the generated content:
  - `<!-- AVAILABLE-TOOLSETS-START -->` / `<!-- AVAILABLE-TOOLSETS-END -->` (toolset summary table)
  - `<!-- AVAILABLE-TOOLSETS-TOOLS-START -->` / `<!-- AVAILABLE-TOOLSETS-TOOLS-END -->` (tool details)
  - `<!-- AVAILABLE-TOOLSETS-PROMPTS-START -->` / `<!-- AVAILABLE-TOOLSETS-PROMPTS-END -->` (prompt details)
  - `<!-- AVAILABLE-TOOLSETS-RESOURCES-START -->` / `<!-- AVAILABLE-TOOLSETS-RESOURCES-END -->` (resource details)
  - `<!-- AVAILABLE-TOOLSETS-RESOURCES-TEMPLATES-START -->` / `<!-- AVAILABLE-TOOLSETS-RESOURCES-TEMPLATES-END -->` (resource template details)

## Distribution Methods

The server is distributed as a binary executable, a Docker image, an npm package, and a Python package.

- **Native binaries** for Linux, macOS, and Windows are available in the GitHub releases.
- A **container image** (Docker) is built and pushed to the `quay.io/containers/kubernetes_mcp_server` repository.
- An **npm** package is available at [npmjs.com](https://www.npmjs.com/package/kubernetes-mcp-server).
  It wraps the platform-specific binary and provides a convenient way to run the server using `npx`.
- A **Python** package is available at [pypi.org](https://pypi.org/project/kubernetes-mcp-server/).
  It provides a script that downloads the correct platform binary from the GitHub releases page and runs it.
  It provides a convenient way to run the server using `uvx` or `python -m kubernetes_mcp_server`.
