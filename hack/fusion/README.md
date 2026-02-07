# IBM Fusion Automation Scripts

This directory contains automation scripts for managing the IBM Fusion fork of kubernetes-mcp-server.

**Maintainer:** Sandeep Bazar ([@sandeepbazar](https://github.com/sandeepbazar))

## Scripts

### sync_upstream_and_apply_fusion_patch.py

**Purpose:** Automate the process of syncing with upstream and applying Fusion integration hooks.

**Language:** Python 3

**Features:**
- Idempotent patch application (no duplicates)
- Cross-platform (macOS, Linux, Windows/WSL)
- Colored terminal output
- Comprehensive error handling
- Automatic test execution
- Git commit and push automation

**Usage:**
```bash
# From repository root
python3 hack/fusion/sync_upstream_and_apply_fusion_patch.py

# Force sync even with uncommitted changes
python3 hack/fusion/sync_upstream_and_apply_fusion_patch.py --force
```

**What it does:**
1. Verifies git working tree is clean
2. Checks git remotes (adds upstream if missing)
3. Fetches and merges upstream/main
4. Applies Fusion patches to:
   - `pkg/toolsets/toolsets.go` (registration hook)
   - `pkg/mcp/modules.go` (fusion import)
5. Runs `go test ./...`
6. Commits and pushes changes

**Exit codes:**
- `0` - Success
- `1` - Error (with descriptive message)

### sync.sh

**Purpose:** Convenient wrapper for the Python sync script.

**Language:** Bash

**Usage:**
```bash
# From repository root
./hack/fusion/sync.sh

# With force flag
./hack/fusion/sync.sh --force
```

**What it does:**
- Locates the Python script
- Makes it executable if needed
- Passes all arguments to Python script
- Provides cleaner command-line interface

## Patch Logic

### Idempotent Design

Both patches are designed to be idempotent:

#### pkg/toolsets/toolsets.go
- **Detection:** Searches for `registerFusionTools` and `SetFusionRegistration` in file content
- **If found:** Skips patching (prints success message)
- **If not found:** Appends Fusion hook code at end of file

**Code added:**
```go
func init() {
	// IBM Fusion extension integration point
	registerFusionTools()
}

var registerFusionTools = func() {}

func SetFusionRegistration(fn func()) {
	registerFusionTools = fn
}
```

#### pkg/mcp/modules.go
- **Detection:** Searches for `pkg/toolsets/fusion` in file content
- **If found:** Skips patching (prints success message)
- **If not found:** Inserts import in alphabetical order

**Code added:**
```go
_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/fusion"
```

**Insertion logic:**
- Finds import block
- Inserts after `pkg/toolsets/core`
- Maintains alphabetical order
- Preserves formatting

### Safety Features

1. **No duplicates:** Checks for existing code before patching
2. **Fail-fast:** Exits immediately on errors with clear messages
3. **Validation:** Runs tests before committing
4. **Rollback-friendly:** All changes are committed, easy to revert
5. **Cross-platform:** Works on macOS, Linux, Windows/WSL

## Development

### Testing the Script

```bash
# Dry run (check without making changes)
# Note: Script doesn't have dry-run mode yet, but you can:
git stash  # Save current changes
./hack/fusion/sync.sh
git reset --hard HEAD~1  # Undo if needed
git stash pop  # Restore changes
```

### Modifying the Script

When updating the automation:

1. **Test thoroughly** on a clean clone
2. **Verify idempotency** (run twice, second run should be no-op)
3. **Test error cases** (missing remotes, merge conflicts, test failures)
4. **Update documentation** in README.FUSION.md
5. **Commit with clear message**

### Adding New Patches

To add a new integration point:

1. Add detection logic (check if code exists)
2. Add insertion logic (where to add code)
3. Make it idempotent (no duplicates)
4. Add error handling (fail if file structure changed)
5. Update documentation

## Troubleshooting

### Script fails with "File not found"

**Cause:** Upstream structure changed significantly

**Solution:**
1. Review upstream changes manually
2. Update Fusion integration hooks manually
3. Update script to match new structure
4. Submit PR with updated automation

### Script creates duplicate code

**Cause:** Detection logic failed

**Solution:**
1. Report issue with file content
2. Manually remove duplicates
3. Fix detection logic in script
4. Test thoroughly before committing

### Tests fail after sync

**Cause:** Upstream API changes broke Fusion code

**Solution:**
1. Review test failures
2. Update Fusion code to match new APIs
3. Run tests locally until passing
4. Commit fixes
5. Re-run sync script

## Contributing

When contributing to these scripts:

1. **Maintain idempotency** - Scripts must be safe to run multiple times
2. **Add error handling** - Fail fast with clear error messages
3. **Update documentation** - Keep README.FUSION.md in sync
4. **Test cross-platform** - Verify on macOS and Linux
5. **Follow Python style** - PEP 8 for Python code
6. **Use POSIX shell** - For shell scripts (avoid bash-isms)

## License

Same license as the main repository. See [LICENSE](../../LICENSE).