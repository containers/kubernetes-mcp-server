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

### Testing Patterns and Guidelines

This project follows specific testing patterns to ensure consistency, maintainability, and quality. When writing tests, adhere to the following guidelines:

#### Test Framework

- **Use `testify/suite`** for organizing tests into suites
- Tests should be structured using test suites that embed `suite.Suite`
- Each test file should have a corresponding suite struct (e.g., `UnstructuredSuite`, `KubevirtSuite`)
- Use the `suite.Run()` function to execute test suites

Example:
```go
type MyTestSuite struct {
    suite.Suite
}

func (s *MyTestSuite) TestSomething() {
    s.Run("descriptive scenario name", func() {
        // test implementation
    })
}

func TestMyFeature(t *testing.T) {
    suite.Run(t, new(MyTestSuite))
}
```

#### Behavior-Based Testing

- **Test the public API only** - tests should be black-box and not access internal/private functions
- **No mocks** - use real implementations and integration testing where possible
- **Behavior over implementation** - test what the code does, not how it does it
- Focus on observable behavior and outcomes rather than internal state

#### Test Organization

- **Use nested subtests** with `s.Run()` to organize related test cases
- **Descriptive names** - subtest names should clearly describe the scenario being tested
- Group related scenarios together under a parent test (e.g., "edge cases", "with valid input")

Example structure:
```go
func (s *MySuite) TestFeature() {
    s.Run("valid input scenarios", func() {
        s.Run("handles simple case correctly", func() {
            // test code
        })
        s.Run("handles complex case with nested data", func() {
            // test code
        })
    })
    s.Run("edge cases", func() {
        s.Run("returns error for nil input", func() {
            // test code
        })
        s.Run("handles empty input gracefully", func() {
            // test code
        })
    })
}
```

#### Assertions

- **One assertion per test case** - each `s.Run()` block should ideally test one specific behavior
- Use `testify` assertion methods: `s.Equal()`, `s.True()`, `s.False()`, `s.Nil()`, `s.NotNil()`, etc.
- Provide clear assertion messages when the failure reason might not be obvious

Example:
```go
s.Run("returns expected value", func() {
    result := functionUnderTest()
    s.Equal("expected", result, "function should return the expected string")
})
```

#### Coverage

- **Aim for high test coverage** of the public API
- Add edge case tests to cover error paths and boundary conditions
- Common edge cases to consider:
  - Nil/null inputs
  - Empty strings, slices, maps
  - Negative numbers or invalid indices
  - Type mismatches
  - Malformed input (e.g., invalid paths, formats)

#### Error Handling

- **Never ignore errors** in production code
- Always check and handle errors from functions that return them
- In tests, use `s.Require().NoError(err)` for operations that must succeed for the test to be valid
- Use `s.Error(err)` or `s.NoError(err)` for testing error conditions

Example:
```go
s.Run("returns error for invalid input", func() {
    result, err := functionUnderTest(invalidInput)
    s.Error(err, "expected error for invalid input")
    s.Nil(result, "result should be nil when error occurs")
})
```

#### Test Helpers

- Create reusable test helpers in `internal/test/` for common testing utilities
- Test helpers should be generic and reusable across multiple test files
- Document test helpers with clear godoc comments explaining their purpose and usage

Example from this project:
```go
// FieldString retrieves a string field from an unstructured object using JSONPath-like notation.
// Examples:
//   - "spec.runStrategy"
//   - "spec.template.spec.volumes[0].containerDisk.image"
func FieldString(obj *unstructured.Unstructured, path string) string {
    // implementation
}
```

#### Examples from the Codebase

Good examples of these patterns can be found in:
- `internal/test/unstructured_test.go` - demonstrates proper use of testify/suite, nested subtests, and edge case testing
- `pkg/mcp/kubevirt_test.go` - shows behavior-based testing of the MCP layer
- `pkg/kubernetes/manager_test.go` - illustrates testing with proper setup/teardown and subtests

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

- Go modules target Go **1.25** (see `go.mod`).
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
