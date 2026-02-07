# IBM Fusion MCP Server - Quick Start

> **📖 For complete documentation, see [README.FUSION.md](../../README.FUSION.md) at the repository root.**

This is the quick start guide for IBM Fusion extensions to kubernetes-mcp-server.

## What is IBM Fusion MCP Server?

IBM Fusion MCP Server is a fork of [containers/kubernetes-mcp-server](https://github.com/containers/kubernetes-mcp-server) that adds specialized tools for managing IBM Fusion and OpenShift environments.

**Upstream:** https://github.com/containers/kubernetes-mcp-server

## Quick Start

### 1. Enable Fusion Tools

Fusion tools are **disabled by default**. Enable them with an environment variable:

```bash
export FUSION_TOOLS_ENABLED=true
```

### 2. Run the Server

```bash
# Build and run locally
make build
FUSION_TOOLS_ENABLED=true ./kubernetes-mcp-server

# Or use package managers
FUSION_TOOLS_ENABLED=true npx -y kubernetes-mcp-server@latest
FUSION_TOOLS_ENABLED=true uvx kubernetes-mcp-server@latest
```

### 3. Verify Fusion Tools are Loaded

Check the logs for:

```
I0207 12:00:00.000000   12345 registry.go:18] Registering IBM Fusion toolset
```

## Available Tools

### Storage Domain

#### `fusion.storage.summary`

Get comprehensive storage status for IBM Fusion/OpenShift environments.

**Input:** None (empty object `{}`)

**Output:**
```json
{
  "summary": {
    "storageClasses": [
      {
        "name": "gp2",
        "provisioner": "kubernetes.io/aws-ebs",
        "isDefault": true
      }
    ],
    "pvcStats": {
      "bound": 15,
      "pending": 2,
      "lost": 0,
      "total": 17
    },
    "odfInstalled": true
  }
}
```

**Features:**
- Lists all storage classes with provisioner and default status
- Provides PVC statistics by phase (Bound, Pending, Lost)
- Detects ODF/OCS installation

## Architecture Overview

### Directory Structure

```
ibm-fusion-mcp-server/
├── internal/fusion/          # Internal implementation
│   ├── config/              # Feature gate configuration
│   ├── clients/             # Kubernetes client wrappers
│   └── services/            # Domain logic (storage, compute, network)
├── pkg/toolsets/fusion/     # Public toolset API
│   ├── registry.go          # Registration hook
│   ├── toolset.go           # Toolset implementation
│   └── storage/             # Storage domain tools
└── docs/fusion/             # This documentation
```

### Integration Points

Fusion integrates with upstream via **two minimal touchpoints**:

1. **`pkg/toolsets/toolsets.go`** - Single registration hook (11 lines)
2. **`pkg/mcp/modules.go`** - Fusion package import (1 line)

This minimal integration ensures:
- Easy upstream syncing
- No merge conflicts
- Clean separation of concerns

### Feature Gating

When `FUSION_TOOLS_ENABLED` is not set or set to `false`:
- Fusion tools are not registered
- Server behaves identically to upstream
- Zero Fusion overhead

## Development

### Adding New Tools

See the complete guide in [README.FUSION.md - How to Add the Next Fusion Tool](../../README.FUSION.md#how-to-add-the-next-fusion-tool)

**Quick steps:**

1. Create tool in `pkg/toolsets/fusion/<domain>/tool_<name>.go`
2. Add service logic in `internal/fusion/services/<domain>.go`
3. Register in `pkg/toolsets/fusion/toolset.go`
4. Add tests
5. Update documentation

**Naming convention:** `fusion.<domain>.<action>`

### Testing

```bash
# Test Fusion code only
go test ./internal/fusion/... ./pkg/toolsets/fusion/...

# Test everything
make test

# Test with coverage
go test -cover ./internal/fusion/... ./pkg/toolsets/fusion/...
```

### Building

```bash
# Build binary
make build

# Build for all platforms
make build-all-platforms
```

## Upstream Sync

For detailed upstream sync procedures, see [README.FUSION.md - Upstream Sync SOP](../../README.FUSION.md#upstream-sync-sop)

**Quick sync:**

```bash
# Commit Fusion changes first!
git add . && git commit -m "feat(fusion): latest changes"
git push origin fusion-main

# Sync with upstream
git fetch upstream
git merge upstream/main

# Resolve conflicts (Fusion files = keep ours)
# Push updated branch
git push origin fusion-main
```

## Tool Catalog

### Current Tools

| Tool | Domain | Status |
|------|--------|--------|
| `fusion.storage.summary` | Storage | ✅ Implemented |

### Planned Domains

1. **Storage** - PVC management, ODF/OCS integration
2. **Compute** - Node management, workload optimization
3. **Network** - Network policies, service mesh
4. **Backup/Restore** - Disaster recovery, snapshots
5. **HCP** - Hosted Control Planes management
6. **Virtualization** - KubeVirt integration

## Documentation

- **[README.FUSION.md](../../README.FUSION.md)** - 📖 Complete operations runbook (START HERE)
- **[This file](README.md)** - Quick start guide
- **[ARCHITECTURE.md](ARCHITECTURE.md)** - Detailed architecture (future)
- **[ROADMAP.md](ROADMAP.md)** - Feature roadmap (future)
- **[TOOLS.md](TOOLS.md)** - Complete tool reference (future)

## Support

- **Fusion-specific issues:** File in this repository with `fusion` label
- **Upstream issues:** File in [containers/kubernetes-mcp-server](https://github.com/containers/kubernetes-mcp-server/issues)

## Contributing

See the [Contribution Style Guide](../../README.FUSION.md#how-to-add-the-next-fusion-tool) in README.FUSION.md.

**Key principles:**
- Keep Fusion code isolated in `internal/fusion/` and `pkg/toolsets/fusion/`
- Do not refactor upstream code
- Add tests for all new functionality
- Follow existing patterns from upstream toolsets
- Update documentation

## License

Same license as upstream kubernetes-mcp-server. See [LICENSE](../../LICENSE).

---

**For complete documentation, operational procedures, and troubleshooting, see [README.FUSION.md](../../README.FUSION.md).**