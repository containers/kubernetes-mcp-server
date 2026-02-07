# IBM Fusion MCP Server - Operations Runbook

**Canonical reference for managing the IBM Fusion fork of kubernetes-mcp-server**

## What is This Repository?

This is **ibm-fusion-mcp-server**, a fork of [containers/kubernetes-mcp-server](https://github.com/containers/kubernetes-mcp-server) that adds IBM Fusion-specific MCP tool extensions.

**Upstream:** https://github.com/containers/kubernetes-mcp-server

**Purpose:** Provide specialized tools for managing IBM Fusion and OpenShift environments across multiple domains:
- Storage (PVC, storage classes, ODF/OCS)
- Compute (node management, workload optimization)
- Network (network policies, service mesh)
- Backup/Restore (disaster recovery, snapshots)
- HCP (Hosted Control Planes, multi-tenant management)
- Virtualization (KubeVirt integration, VM lifecycle)

## Design Goals and Non-Goals

### Goals ✅

1. **Keep upstream clean** - Minimize modifications to upstream code
2. **Isolate Fusion changes** - All Fusion code lives in dedicated directories
3. **Feature gating** - Fusion tools disabled by default, enabled via `FUSION_TOOLS_ENABLED=true`
4. **Maintain sync-ability** - Regular upstream syncs with minimal conflicts
5. **Production-ready** - Well-tested, documented, and maintainable

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
│   │   └── kubernetes.go              # K8s client wrappers
│   └── services/
│       └── storage.go                 # Storage domain logic
│
├── pkg/toolsets/fusion/                # 🔓 Public Fusion toolset API
│   ├── registry.go                    # Toolset registration hook
│   ├── toolset.go                     # Toolset implementation
│   └── storage/
│       ├── tool_storage_summary.go    # First tool: fusion.storage.summary
│       └── types.go                   # Input/output types
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

## Upstream Sync SOP

### Recommended Branch Model

```
main          → Tracks upstream (clean sync point)
fusion-main   → Contains all Fusion changes (working branch)
```

**Rationale:** Keep `main` as a clean upstream mirror for easy syncing, do all Fusion work in `fusion-main`.

### Step-by-Step Sync Commands

#### Initial Setup (One-Time)

```bash
# Clone the fork
git clone https://github.com/your-org/ibm-fusion-mcp-server.git
cd ibm-fusion-mcp-server

# Add upstream remote
git remote add upstream https://github.com/containers/kubernetes-mcp-server.git
git fetch upstream

# Verify remotes
git remote -v
# origin    https://github.com/your-org/ibm-fusion-mcp-server.git (fetch)
# upstream  https://github.com/containers/kubernetes-mcp-server.git (fetch)
```

#### Regular Sync (Recommended: Weekly or Monthly)

**⚠️ CRITICAL: Always commit and push Fusion changes before syncing!**

```bash
# 1. Ensure all Fusion changes are committed
git status
git add .
git commit -m "feat(fusion): latest changes before upstream sync"
git push origin fusion-main

# 2. Switch to main branch and sync with upstream
git checkout main
git fetch upstream
git merge upstream/main
# Or use rebase if you prefer: git rebase upstream/main

# 3. Push updated main to fork
git push origin main

# 4. Merge updated main into fusion-main
git checkout fusion-main
git merge main
# Or use rebase: git rebase main

# 5. Resolve conflicts if any (see conflict resolution checklist below)

# 6. Push updated fusion-main
git push origin fusion-main
```

#### Alternative: Direct Merge (Simpler, but less clean)

```bash
# 1. Ensure all Fusion changes are committed
git status
git add .
git commit -m "feat(fusion): latest changes before upstream sync"
git push origin fusion-main

# 2. Fetch and merge upstream directly into fusion-main
git fetch upstream
git merge upstream/main

# 3. Resolve conflicts if any

# 4. Push updated fusion-main
git push origin fusion-main
```

### Conflict Resolution Checklist

When conflicts occur during sync, follow this priority:

#### ✅ Always Keep (Ours)
- [ ] `internal/fusion/**` - All Fusion internal code
- [ ] `pkg/toolsets/fusion/**` - All Fusion toolset code
- [ ] `docs/fusion/**` - All Fusion documentation
- [ ] `README.FUSION.md` - This file

#### ⚖️ Carefully Merge (Both)
- [ ] `pkg/toolsets/toolsets.go` - Preserve the Fusion hook (lines 53-63)
- [ ] `pkg/mcp/modules.go` - Preserve the Fusion import (line 6)
- [ ] `go.mod` / `go.sum` - Run `go mod tidy` after merge
- [ ] `README.md` - Keep the 1-line Fusion pointer at top

#### ⬆️ Prefer Upstream (Theirs)
- [ ] All other files - Accept upstream changes unless Fusion requires modification

#### Conflict Resolution Commands

```bash
# For files to keep ours
git checkout --ours path/to/file
git add path/to/file

# For files to keep theirs
git checkout --theirs path/to/file
git add path/to/file

# For files needing manual merge
# Edit the file, resolve conflicts, then:
git add path/to/file

# After resolving all conflicts
git commit -m "chore: merge upstream/main, resolved conflicts"
```

### Never Lose Changes Rule

**Before any sync operation:**

1. ✅ Commit all Fusion changes: `git commit -am "wip: before sync"`
2. ✅ Push to remote: `git push origin fusion-main`
3. ✅ Verify push succeeded: `git log origin/fusion-main`

**If sync goes wrong:**

```bash
# Reset to last known good state
git reset --hard origin/fusion-main

# Or create a backup branch first
git branch fusion-main-backup
git push origin fusion-main-backup
```

## Post-Sync Validation Checklist

After every upstream sync, validate the fork:

### 1. Dependencies and Build

```bash
# Resolve dependencies
go mod tidy

# Verify no missing dependencies
go mod verify

# Build the binary
make build
# Or: go build -o kubernetes-mcp-server ./cmd/kubernetes-mcp-server

# Check binary exists
ls -lh kubernetes-mcp-server
```

### 2. Run Tests

```bash
# Test all code
go test ./...

# Test only Fusion code
go test ./internal/fusion/... ./pkg/toolsets/fusion/...

# Test with coverage
go test -cover ./internal/fusion/... ./pkg/toolsets/fusion/...
```

### 3. Run Server - Fusion Disabled (Default)

```bash
# Start server with Fusion tools disabled
./kubernetes-mcp-server

# In another terminal, verify Fusion tools are NOT loaded
# Check logs for absence of: "Registering IBM Fusion toolset"

# List tools (should not include fusion.* tools)
# If server supports --list-tools flag:
./kubernetes-mcp-server --list-tools | grep fusion
# Should return nothing
```

### 4. Run Server - Fusion Enabled

```bash
# Start server with Fusion tools enabled
FUSION_TOOLS_ENABLED=true ./kubernetes-mcp-server

# Verify Fusion tools ARE loaded
# Check logs for: "Registering IBM Fusion toolset"

# List tools (should include fusion.* tools)
FUSION_TOOLS_ENABLED=true ./kubernetes-mcp-server --list-tools | grep fusion
# Should show: fusion.storage.summary
```

### 5. Quick Tool Test (fusion.storage.summary)

**Using MCP Inspector:**

```bash
# Install MCP inspector
npm install -g @modelcontextprotocol/inspector

# Run server with inspector
FUSION_TOOLS_ENABLED=true npx @modelcontextprotocol/inspector $(pwd)/kubernetes-mcp-server

# In the inspector UI:
# 1. Connect to the server
# 2. Find "fusion.storage.summary" in tools list
# 3. Execute with empty arguments: {}
# 4. Verify output contains: storageClasses, pvcStats, odfInstalled
```

**Manual Test (if you have a test cluster):**

```bash
# Ensure you have a kubeconfig pointing to a test cluster
export KUBECONFIG=~/.kube/config

# Run server in STDIO mode
FUSION_TOOLS_ENABLED=true ./kubernetes-mcp-server

# Send MCP request (example using echo and jq)
echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"fusion.storage.summary","arguments":{}}}' | \
  FUSION_TOOLS_ENABLED=true ./kubernetes-mcp-server | jq .

# Expected output structure:
# {
#   "summary": {
#     "storageClasses": [...],
#     "pvcStats": {"bound": N, "pending": N, "lost": N, "total": N},
#     "odfInstalled": true/false
#   }
# }
```

### 6. Validation Checklist Summary

- [ ] `go mod tidy` succeeds
- [ ] `make build` succeeds
- [ ] `go test ./...` passes
- [ ] Server runs with Fusion disabled (no fusion.* tools)
- [ ] Server runs with Fusion enabled (fusion.* tools present)
- [ ] `fusion.storage.summary` tool executes successfully
- [ ] No regression in upstream functionality

## Tool Catalog

### Current Tools (Implemented)

| Tool Name | Domain | Description | Status |
|-----------|--------|-------------|--------|
| `fusion.storage.summary` | Storage | Get storage status: classes, PVC stats, ODF detection | ✅ Implemented |

### Planned Tool Domains

1. **Storage** (In Progress)
   - `fusion.storage.summary` ✅
   - `fusion.storage.pvc.list` (planned)
   - `fusion.storage.pvc.resize` (planned)
   - `fusion.storage.odf.status` (planned)

2. **Compute** (Planned)
   - `fusion.compute.node.list`
   - `fusion.compute.node.cordon`
   - `fusion.compute.node.drain`
   - `fusion.compute.workload.optimize`

3. **Network** (Planned)
   - `fusion.network.policy.list`
   - `fusion.network.policy.create`
   - `fusion.network.servicemesh.status`

4. **Backup/Restore** (Planned)
   - `fusion.backup.create`
   - `fusion.backup.list`
   - `fusion.restore.execute`

5. **HCP (Hosted Control Planes)** (Planned)
   - `fusion.hcp.list`
   - `fusion.hcp.create`
   - `fusion.hcp.status`

6. **Virtualization** (Planned)
   - `fusion.vm.list`
   - `fusion.vm.create`
   - `fusion.vm.start`
   - `fusion.vm.stop`

## How to Add the Next Fusion Tool

### Example: Adding `fusion.storage.pvc.list`

#### 1. Create Tool Implementation

**File:** `pkg/toolsets/fusion/storage/tool_pvc_list.go`

```go
package storage

import (
    "encoding/json"
    "github.com/containers/kubernetes-mcp-server/pkg/api"
    "github.com/google/jsonschema-go/jsonschema"
    "k8s.io/utils/ptr"
)

func InitPVCList() api.ServerTool {
    return api.ServerTool{
        Tool: api.Tool{
            Name:        "fusion.storage.pvc.list",
            Description: "List PersistentVolumeClaims with filtering options",
            Annotations: api.ToolAnnotations{
                Title:        "IBM Fusion PVC List",
                ReadOnlyHint: ptr.To(true),
            },
            InputSchema: &jsonschema.Schema{
                Type: jsonschema.Type{jsonschema.TypeObject},
                Properties: map[string]*jsonschema.Schema{
                    "namespace": {
                        Type:        jsonschema.Type{jsonschema.TypeString},
                        Description: "Namespace to list PVCs from (empty for all)",
                    },
                },
            },
        },
        Handler: handlePVCList,
    }
}

func handlePVCList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
    // Implementation here
    // ...
    return api.NewToolCallResult(jsonOutput, nil), nil
}
```

#### 2. Add Service Logic (if needed)

**File:** `internal/fusion/services/storage.go`

```go
// Add new method to StorageService
func (s *StorageService) ListPVCs(ctx context.Context, namespace string) (*PVCList, error) {
    // Implementation
}
```

#### 3. Register Tool in Toolset

**File:** `pkg/toolsets/fusion/toolset.go`

```go
func (t *Toolset) GetTools(o api.Openshift) []api.ServerTool {
    return []api.ServerTool{
        storage.InitStorageSummary(),
        storage.InitPVCList(),  // ← Add this line
    }
}
```

#### 4. Add Tests

**File:** `pkg/toolsets/fusion/storage/tool_pvc_list_test.go`

```go
package storage

import (
    "testing"
    "github.com/stretchr/testify/suite"
)

type PVCListSuite struct {
    suite.Suite
}

func (s *PVCListSuite) TestPVCList() {
    // Test implementation
}

func TestPVCListSuite(t *testing.T) {
    suite.Run(t, new(PVCListSuite))
}
```

#### 5. Update Documentation

**File:** `docs/fusion/README.md`

Add to the "Available Tools" section:

```markdown
- **`fusion.storage.pvc.list`** - List PersistentVolumeClaims
  - Filter by namespace
  - Returns PVC name, status, capacity, storage class
```

**File:** `README.FUSION.md` (this file)

Update the "Current Tools" table.

#### 6. Naming Convention

All Fusion tools must follow this pattern:

```
fusion.<domain>.<action>
```

Examples:
- ✅ `fusion.storage.summary`
- ✅ `fusion.storage.pvc.list`
- ✅ `fusion.compute.node.drain`
- ✅ `fusion.network.policy.create`
- ❌ `storage.fusion.summary` (wrong order)
- ❌ `fusion-storage-summary` (wrong separator)

#### 7. Testing Checklist

Before committing a new tool:

- [ ] Tool implementation in `pkg/toolsets/fusion/<domain>/`
- [ ] Service logic in `internal/fusion/services/` (if needed)
- [ ] Tool registered in `pkg/toolsets/fusion/toolset.go`
- [ ] Unit tests added and passing
- [ ] Tool appears in list when `FUSION_TOOLS_ENABLED=true`
- [ ] Tool executes successfully with valid input
- [ ] Tool handles errors gracefully
- [ ] Documentation updated in `docs/fusion/README.md`
- [ ] This file's tool catalog updated

## Quick Reference Commands

```bash
# Build
make build

# Test
go test ./...

# Test Fusion only
go test ./internal/fusion/... ./pkg/toolsets/fusion/...

# Run with Fusion disabled
./kubernetes-mcp-server

# Run with Fusion enabled
FUSION_TOOLS_ENABLED=true ./kubernetes-mcp-server

# Sync with upstream
git fetch upstream && git merge upstream/main

# Format code
go fmt ./internal/fusion/... ./pkg/toolsets/fusion/...

# Lint
make lint
```

## Support and Troubleshooting

### Common Issues

**Issue:** Fusion tools not appearing after enabling

**Solution:**
```bash
# Verify environment variable is set
echo $FUSION_TOOLS_ENABLED

# Check logs for registration message
FUSION_TOOLS_ENABLED=true ./kubernetes-mcp-server 2>&1 | grep -i fusion

# Rebuild to ensure latest code
make clean && make build
```

**Issue:** Merge conflicts during upstream sync

**Solution:**
1. Follow the conflict resolution checklist above
2. For Fusion files, always keep "ours"
3. For integration points, carefully preserve the hook
4. Run `go mod tidy` after resolving go.mod conflicts

**Issue:** Tests failing after upstream sync

**Solution:**
```bash
# Update dependencies
go mod tidy

# Check for API changes in upstream
git diff upstream/main..HEAD -- pkg/api/

# Update Fusion code to match new upstream APIs
```

## Maintenance Schedule

- **Weekly:** Check for upstream updates
- **Monthly:** Perform upstream sync
- **Quarterly:** Review and update documentation
- **As needed:** Add new Fusion tools based on requirements

## Version History

- **v1.0.0** - Initial Fusion fork with storage.summary tool
- **v1.1.0** - (Planned) Add compute domain tools
- **v1.2.0** - (Planned) Add network domain tools

---

**Last Updated:** 2026-02-07  
**Maintainer:** IBM Fusion Team  
**Upstream Version:** Synced with containers/kubernetes-mcp-server main branch