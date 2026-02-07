# IBM Fusion MCP Server - Operations Runbook

**Canonical reference for managing the IBM Fusion fork of kubernetes-mcp-server**

**Maintainer:** Sandeep Bazar  
**GitHub:** [@sandeepbazar](https://github.com/sandeepbazar)  
**Repository:** [ibm-fusion-mcp-server](https://github.com/sandeepbazar/ibm-fusion-mcp-server)  
**Last Updated:** 2026-02-07  
**Upstream Version:** Synced with containers/kubernetes-mcp-server main branch

## What is This Repository?

This is **ibm-fusion-mcp-server**, a fork of [containers/kubernetes-mcp-server](https://github.com/containers/kubernetes-mcp-server) that adds IBM Fusion-specific MCP tool extensions with **multi-cluster and fleet management** capabilities.

**Upstream:** https://github.com/containers/kubernetes-mcp-server

**Purpose:** Provide specialized tools for managing IBM Fusion and OpenShift environments across **single clusters, multiple clusters, and entire fleets**:

## IBM Fusion Architecture Mapping

IBM Fusion is organized into service domains (blue services) that this MCP server supports:

### Fusion Data Services
- **Data Foundation** - OpenShift Data Foundation (ODF/OCS) for persistent storage
- **Global Data Platform (GDP)** - IBM Spectrum Scale integration for high-performance file storage
- **Backup & Restore** - OADP/Velero-based backup and disaster recovery
- **Disaster Recovery** - Metro DR and Regional DR for business continuity
- **Data Cataloging** - Metadata management and data discovery
- **Content Aware Storage (CAS)** - Intelligent storage tiering and optimization

### Fusion Base
- **Serviceability** - Must-gather, logging, and diagnostic tools
- **Observability** - Prometheus, Grafana, OpenTelemetry integration

### Additional Capabilities
- **Virtualization** - KubeVirt/OpenShift Virtualization for VM workloads
- **Hosted Control Planes (HCP)** - HyperShift for multi-tenant cluster management

## Design Goals and Non-Goals

### Goals ✅

1. **Keep upstream clean** - Minimize modifications to upstream code
2. **Isolate Fusion changes** - All Fusion code lives in dedicated directories
3. **Feature gating** - Fusion tools disabled by default, enabled via `FUSION_TOOLS_ENABLED=true`
4. **Multi-cluster support** - Single cluster, multi-cluster, and fleet targeting
5. **Maintain sync-ability** - Regular upstream syncs with minimal conflicts
6. **Production-ready** - Well-tested, documented, and maintainable

### Non-Goals ❌

1. **No upstream PRs** - Fusion extensions are specific to IBM needs, not intended for upstream contribution
2. **No upstream refactoring** - We adapt to upstream patterns, not change them
3. **No breaking changes** - Fork must work exactly like upstream when Fusion tools are disabled

## Directory Structure

```
ibm-fusion-mcp-server/
├── README.md                           # Upstream README (+ 1 line pointer to this file)
├── README.FUSION.md                    # ⭐ This file - canonical Fusion reference
│
├── internal/fusion/                    # 🔒 Internal Fusion implementation
│   ├── config/
│   │   ├── config.go                  # Feature gate (FUSION_TOOLS_ENABLED)
│   │   └── config_test.go             # Config tests
│   ├── clients/
│   │   ├── kubernetes.go              # K8s client wrappers
│   │   └── registry.go                # Multi-cluster client registry
│   ├── services/
│   │   ├── common.go                  # Shared service utilities
│   │   ├── storage.go                 # Storage domain logic
│   │   ├── datafoundation.go          # Data Foundation logic
│   │   ├── backup.go                  # Backup & Restore logic
│   │   └── multidom.go                # Multi-domain services
│   └── targeting/
│       └── target.go                  # Multi-cluster targeting model
│
├── pkg/toolsets/fusion/                # 🔓 Public Fusion toolset API
│   ├── registry.go                    # Toolset registration hook
│   ├── toolset.go                     # Toolset implementation
│   ├── storage/
│   │   ├── tool_storage_summary.go    # Storage summary tool
│   │   └── types.go                   # Input/output types
│   ├── datafoundation/
│   │   └── tool_status.go             # Data Foundation status
│   ├── backup/
│   │   └── tool_jobs_list.go          # Backup jobs list
│   └── alltools/
│       └── tools.go                   # All other domain tools
│
├── docs/fusion/                        # 📚 Fusion documentation
│   └── README.md                      # Quick start guide
│
└── [upstream files...]                # All other files from upstream
```

## Integration Points (Upstream Modifications)

We touch **exactly 2 upstream files** to integrate Fusion extensions:

### 1. `pkg/toolsets/toolsets.go` (11 lines added)

**Why:** Single integration hook for Fusion toolset registration

**What we added:**
```go
func init() {
    // IBM Fusion extension integration point
    registerFusionTools()
}

// registerFusionTools is a placeholder that will be implemented by the fusion package
var registerFusionTools = func() {}

// SetFusionRegistration allows the fusion package to hook into the registration process
func SetFusionRegistration(fn func()) {
    registerFusionTools = fn
}
```

**Pattern:** Function variable hook that Fusion package populates via `SetFusionRegistration()`

**Guidance:** All future Fusion integration must use this same hook. Do NOT add new integration points elsewhere.

### 2. `pkg/mcp/modules.go` (1 line added)

**Why:** Import Fusion package so its `init()` function runs

**What we added:**
```go
import (
    _ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/config"
    _ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/core"
    _ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/fusion"  // ← Added this line
    _ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/helm"
    // ... other imports
)
```

**Pattern:** Blank import to trigger package initialization

**Guidance:** This is the only import needed. Do NOT scatter Fusion imports across multiple files.

## Multi-Cluster Architecture

### Targeting Model

Fusion tools support flexible cluster targeting:

| Target Type | Description | Use Case |
|-------------|-------------|----------|
| **single** | Target one specific cluster | Default, most common operations |
| **multi** | Target explicitly named clusters | Coordinated operations across specific clusters |
| **fleet** | Target all clusters in a fleet/hub | Fleet-wide status checks |
| **selector** | Target clusters matching labels | Environment-based operations (prod, dev, etc.) |
| **all** | Target all registered clusters | Global operations |

### Client Registry

The multi-cluster client registry (`internal/fusion/clients/registry.go`) provides:

- **Thread-safe** concurrent operations across clusters
- **Timeout control** per-cluster operation timeouts
- **Graceful failure** handling - one cluster failure doesn't stop others
- **Kubeconfig support** - multiple contexts, in-cluster config
- **Result aggregation** - per-cluster results with summary

## Single Cluster vs Multi-Cluster Usage

### Default Behavior (Single Cluster)

When no `target` is specified, tools operate on the **current kubeconfig context**:

```json
{
  "name": "fusion.datafoundation.status",
  "arguments": {}
}
```

**Response:**
```json
{
  "target": {
    "type": "single"
  },
  "clusterResults": {
    "default": {
      "clusterName": "default",
      "success": true,
      "data": {
        "installed": true,
        "ready": true,
        "namespace": "openshift-storage",
        "storageClasses": ["ocs-storagecluster-ceph-rbd"]
      }
    }
  },
  "summary": {
    "clustersTotal": 1,
    "clustersOk": 1,
    "clustersFailed": 0
  }
}
```

### Single Cluster (Named Context)

Target a specific kubeconfig context:

```json
{
  "name": "fusion.backup.jobs.list",
  "arguments": {
    "target": {
      "type": "single",
      "cluster": "prod-us-east-1"
    }
  }
}
```

### Multi-Cluster (Explicit List)

Target multiple specific clusters:

```json
{
  "name": "fusion.dr.status",
  "arguments": {
    "target": {
      "type": "multi",
      "clusters": ["prod-us-east-1", "prod-us-west-2"]
    }
  }
}
```

**Response:**
```json
{
  "target": {
    "type": "multi",
    "clusters": ["prod-us-east-1", "prod-us-west-2"]
  },
  "clusterResults": {
    "prod-us-east-1": {
      "clusterName": "prod-us-east-1",
      "success": true,
      "data": {
        "installed": true,
        "ready": true,
        "message": "DR CRDs found (Ramen DR)"
      }
    },
    "prod-us-west-2": {
      "clusterName": "prod-us-west-2",
      "success": true,
      "data": {
        "installed": true,
        "ready": true,
        "message": "DR CRDs found (Ramen DR)"
      }
    }
  },
  "summary": {
    "clustersTotal": 2,
    "clustersOk": 2,
    "clustersFailed": 0
  }
}
```

### Fleet (All Clusters)

Target all clusters in your fleet:

```json
{
  "name": "fusion.virtualization.status",
  "arguments": {
    "target": {
      "type": "fleet"
    }
  }
}
```

### Selector (Label-Based)

Target clusters matching criteria:

```json
{
  "name": "fusion.observability.summary",
  "arguments": {
    "target": {
      "type": "selector",
      "selector": "env=prod,region=us"
    }
  }
}
```

## Fleet Admin Scenarios

Real-world scenarios for fleet administrators:

### 1. Check Data Foundation Health Across All Clusters

```json
{
  "name": "fusion.datafoundation.status",
  "arguments": {
    "target": {"type": "fleet"}
  }
}
```

**Use Case:** Morning health check - verify ODF is running on all clusters

### 2. List Backup Jobs Across Prod and DR

```json
{
  "name": "fusion.backup.jobs.list",
  "arguments": {
    "target": {
      "type": "multi",
      "clusters": ["prod-primary", "prod-dr"]
    }
  }
}
```

**Use Case:** Verify backup jobs completed successfully on both sites

### 3. Find Which Clusters Have Virtualization Installed

```json
{
  "name": "fusion.virtualization.status",
  "arguments": {
    "target": {"type": "fleet"}
  }
}
```

**Use Case:** Inventory check - which clusters support VM workloads

### 4. HCP Overview Across Fleet

```json
{
  "name": "fusion.hcp.status",
  "arguments": {
    "target": {"type": "fleet"}
  }
}
```

**Use Case:** Check HyperShift deployment status across management clusters

### 5. DR Readiness Summary for All Clusters

```json
{
  "name": "fusion.dr.status",
  "arguments": {
    "target": {"type": "fleet"}
  }
}
```

**Use Case:** Disaster recovery audit - verify DR is configured everywhere

### 6. CAS Installed Where?

```json
{
  "name": "fusion.cas.status",
  "arguments": {
    "target": {"type": "fleet"}
  }
}
```

**Use Case:** Find clusters with Content Aware Storage deployed

### 7. Cataloging Health Across Fleet

```json
{
  "name": "fusion.catalog.status",
  "arguments": {
    "target": {"type": "fleet"}
  }
}
```

**Use Case:** Verify data cataloging services are operational

### 8. Storage Classes Drift Across Clusters

```json
{
  "name": "fusion.storage.summary",
  "arguments": {
    "target": {
      "type": "multi",
      "clusters": ["prod-1", "prod-2", "prod-3"]
    }
  }
}
```

**Use Case:** Compare storage class configurations for consistency

### 9. PVC Pending Hotspots Per Cluster

```json
{
  "name": "fusion.storage.summary",
  "arguments": {
    "target": {"type": "fleet"}
  }
}
```

**Use Case:** Identify clusters with pending PVCs (storage issues)

### 10. Observability Stack Status Per Cluster

```json
{
  "name": "fusion.observability.summary",
  "arguments": {
    "target": {"type": "fleet"}
  }
}
```

**Use Case:** Verify Prometheus/Grafana/OTEL are running everywhere

## Tool Catalog

### Implemented Tools ✅

| Tool Name | Domain | Description | Multi-Cluster |
|-----------|--------|-------------|---------------|
| `fusion.storage.summary` | Storage | Storage classes, PVC stats, ODF detection | ✅ |
| `fusion.datafoundation.status` | Data Foundation | ODF/OCS installation and health status | ✅ |
| `fusion.gdp.status` | Global Data Platform | IBM Spectrum Scale/GDP status | ✅ |
| `fusion.backup.jobs.list` | Backup & Restore | List OADP/Velero backup jobs | ✅ |
| `fusion.dr.status` | Disaster Recovery | Metro/Regional DR status | ✅ |
| `fusion.catalog.status` | Data Cataloging | Data catalog service status | ✅ |
| `fusion.cas.status` | Content Aware Storage | CAS deployment status | ✅ |
| `fusion.serviceability.summary` | Serviceability | Must-gather and logging status | ✅ |
| `fusion.observability.summary` | Observability | Prometheus, Grafana, OTEL status | ✅ |
| `fusion.virtualization.status` | Virtualization | KubeVirt/OpenShift Virt status | ✅ |
| `fusion.hcp.status` | Hosted Control Planes | HyperShift/HCP status | ✅ |

### Planned Enhancements 🚧

1. **Storage Domain**
   - `fusion.storage.pvc.list` - List PVCs with filtering
   - `fusion.storage.pvc.resize` - Resize PVC operations
   - `fusion.storage.classes.compare` - Compare storage classes across clusters

2. **Backup Domain**
   - `fusion.backup.policies.list` - List backup policies
   - `fusion.backup.schedule.status` - Check backup schedules

3. **Virtualization Domain**
   - `fusion.vm.list` - List virtual machines
   - `fusion.vm.migrate` - Migrate VMs between nodes

4. **HCP Domain**
   - `fusion.hcp.list` - List hosted clusters
   - `fusion.hcp.nodepool.status` - Check node pool status

## Running the Server

### Local Development (Single Cluster)

```bash
# Build the server
make build

# Run with Fusion tools enabled (uses current kubeconfig context)
FUSION_TOOLS_ENABLED=true ./kubernetes-mcp-server

# Run with MCP Inspector for testing
FUSION_TOOLS_ENABLED=true npx @modelcontextprotocol/inspector $(pwd)/kubernetes-mcp-server
```

### Multi-Cluster Setup

```bash
# Ensure your kubeconfig has multiple contexts
kubectl config get-contexts

# The server will automatically register all contexts
FUSION_TOOLS_ENABLED=true ./kubernetes-mcp-server

# Tools can now target any registered context
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `FUSION_TOOLS_ENABLED` | `false` | Enable Fusion tools |
| `KUBECONFIG` | `~/.kube/config` | Path to kubeconfig file |
| `FUSION_TIMEOUT` | `30` | Default operation timeout (seconds) |

## Troubleshooting

### Issue: "I don't see my changes on GitHub"

**Checklist:**
1. ✅ Verify you're on the correct branch: `git branch`
2. ✅ Verify commit was made: `git log -1`
3. ✅ Verify push succeeded: `git log origin/fusion-tools-v2`
4. ✅ Check GitHub URL uses correct branch: `/blob/fusion-tools-v2/`
5. ✅ Clear browser cache or try incognito mode

### Issue: "Tool returns 'not installed' but component exists"

**Cause:** CRD detection may be failing

**Solution:**
1. Check if CRDs exist: `kubectl get crds | grep <component>`
2. Verify namespace exists: `kubectl get ns | grep <namespace>`
3. Check tool logs for specific error messages

### Issue: "Multi-cluster operation times out"

**Cause:** One or more clusters are slow or unreachable

**Solution:**
1. Increase timeout in target: `"timeout": 60`
2. Check cluster connectivity: `kubectl --context=<cluster> get nodes`
3. Review per-cluster errors in response

### Issue: "Fusion tools not loading"

**Checklist:**
1. ✅ `FUSION_TOOLS_ENABLED=true` is set
2. ✅ Server logs show "Registering IBM Fusion toolset"
3. ✅ Integration hooks are present in `pkg/toolsets/toolsets.go` and `pkg/mcp/modules.go`
4. ✅ Rebuild after changes: `make build`

## Contributing

### Adding a New Tool

See the detailed guide in the original README sections (lines 611-700).

### Testing

```bash
# Test all Fusion code
go test ./internal/fusion/... ./pkg/toolsets/fusion/...

# Test with coverage
go test -cover ./internal/fusion/... ./pkg/toolsets/fusion/...

# Test specific package
go test ./pkg/toolsets/fusion/datafoundation/...
```

### Code Style

- Follow existing patterns in the codebase
- Use `testify/suite` for tests
- Add godoc comments for exported functions
- Keep tools read-only for safety

## Upstream Sync

See the comprehensive upstream sync documentation in the original README (lines 115-453).

**Quick sync:**
```bash
./hack/fusion/sync.sh
```

## Support

**Maintainer:** Sandeep Bazar  
**Email:** sandeep.bazar@in.ibm.com  
**GitHub:** [@sandeepbazar](https://github.com/sandeepbazar)

For issues, please check:
1. This README.FUSION.md
2. docs/fusion/README.md
3. GitHub Issues: https://github.com/sandeepbazar/ibm-fusion-mcp-server/issues

---

**Made with ❤️ by IBM Bob**