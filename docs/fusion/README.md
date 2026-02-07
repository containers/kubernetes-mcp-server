# IBM Fusion MCP Server - Quick Start

**For comprehensive documentation, see [README.FUSION.md](../../README.FUSION.md)**

## What is This?

IBM Fusion MCP Server extends the Kubernetes MCP Server with multi-cluster management capabilities for IBM Fusion environments.

## Quick Start

### 1. Enable Fusion Tools

```bash
export FUSION_TOOLS_ENABLED=true
```

### 2. Build and Run

```bash
# Build
make build

# Run with MCP Inspector
npx @modelcontextprotocol/inspector $(pwd)/kubernetes-mcp-server
```

### 3. Try Your First Tool

In the MCP Inspector, execute:

```json
{
  "name": "fusion.storage.summary",
  "arguments": {}
}
```

## Single Cluster Example

Default behavior - uses current kubeconfig context:

```json
{
  "name": "fusion.datafoundation.status",
  "arguments": {}
}
```

## Multi-Cluster Example

Target multiple clusters explicitly:

```json
{
  "name": "fusion.backup.jobs.list",
  "arguments": {
    "target": {
      "type": "multi",
      "clusters": ["prod-us-east-1", "prod-us-west-2"]
    }
  }
}
```

## Fleet Example

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

## Available Tools

| Tool | Description |
|------|-------------|
| `fusion.storage.summary` | Storage classes, PVC stats, ODF detection |
| `fusion.datafoundation.status` | Data Foundation (ODF/OCS) status |
| `fusion.gdp.status` | Global Data Platform status |
| `fusion.backup.jobs.list` | List backup jobs |
| `fusion.dr.status` | Disaster Recovery status |
| `fusion.catalog.status` | Data Cataloging status |
| `fusion.cas.status` | Content Aware Storage status |
| `fusion.serviceability.summary` | Serviceability tools status |
| `fusion.observability.summary` | Observability stack status |
| `fusion.virtualization.status` | Virtualization status |
| `fusion.hcp.status` | Hosted Control Planes status |

## Response Format

All tools return:

```json
{
  "target": {
    "type": "single|multi|fleet",
    "cluster": "...",
    "clusters": [...]
  },
  "clusterResults": {
    "cluster-name": {
      "clusterName": "cluster-name",
      "success": true,
      "data": { ... }
    }
  },
  "summary": {
    "clustersTotal": 1,
    "clustersOk": 1,
    "clustersFailed": 0
  }
}
```

## Targeting Options

| Type | Description | Example |
|------|-------------|---------|
| `single` | One cluster (default) | `{"type": "single", "cluster": "prod-1"}` |
| `multi` | Specific clusters | `{"type": "multi", "clusters": ["prod-1", "prod-2"]}` |
| `fleet` | All clusters | `{"type": "fleet"}` |
| `selector` | Label-based | `{"type": "selector", "selector": "env=prod"}` |
| `all` | All registered | `{"type": "all"}` |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `FUSION_TOOLS_ENABLED` | `false` | Enable Fusion tools |
| `KUBECONFIG` | `~/.kube/config` | Kubeconfig path |
| `FUSION_TIMEOUT` | `30` | Operation timeout (seconds) |

## Multi-Cluster Setup

Ensure your kubeconfig has multiple contexts:

```bash
# List contexts
kubectl config get-contexts

# The server automatically registers all contexts
# Tools can target any registered context
```

## Troubleshooting

### Tools Not Loading

```bash
# Verify environment variable
echo $FUSION_TOOLS_ENABLED

# Check server logs for "Registering IBM Fusion toolset"
FUSION_TOOLS_ENABLED=true ./kubernetes-mcp-server 2>&1 | grep Fusion
```

### Component Shows "Not Installed"

This is normal if the component isn't deployed. Tools gracefully handle missing CRDs/operators.

### Multi-Cluster Timeout

Increase timeout in target:

```json
{
  "target": {
    "type": "multi",
    "clusters": ["prod-1", "prod-2"],
    "timeout": 60
  }
}
```

## Next Steps

- Read [README.FUSION.md](../../README.FUSION.md) for comprehensive documentation
- Explore fleet admin scenarios
- Learn about upstream sync process
- Contribute new tools

## Support

**Maintainer:** Sandeep Bazar  
**GitHub:** [@sandeepbazar](https://github.com/sandeepbazar)  
**Repository:** [ibm-fusion-mcp-server](https://github.com/sandeepbazar/ibm-fusion-mcp-server)

---

**Made with ❤️ by IBM Bob**