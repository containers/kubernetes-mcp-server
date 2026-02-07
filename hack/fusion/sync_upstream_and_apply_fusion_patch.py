#!/usr/bin/env python3
"""
IBM Fusion MCP Server - Upstream Sync and Patch Automation

This script automates the process of syncing with upstream kubernetes-mcp-server
and ensuring IBM Fusion integration hooks are present.

Author: Sandeep Bazar
GitHub: sandeepbazar
"""

import subprocess
import sys
import os
import re
from pathlib import Path


class Colors:
    """ANSI color codes for terminal output"""
    HEADER = '\033[95m'
    OKBLUE = '\033[94m'
    OKCYAN = '\033[96m'
    OKGREEN = '\033[92m'
    WARNING = '\033[93m'
    FAIL = '\033[91m'
    ENDC = '\033[0m'
    BOLD = '\033[1m'


def print_header(msg):
    print(f"\n{Colors.HEADER}{Colors.BOLD}{'='*70}{Colors.ENDC}")
    print(f"{Colors.HEADER}{Colors.BOLD}{msg}{Colors.ENDC}")
    print(f"{Colors.HEADER}{Colors.BOLD}{'='*70}{Colors.ENDC}\n")


def print_success(msg):
    print(f"{Colors.OKGREEN}✓ {msg}{Colors.ENDC}")


def print_info(msg):
    print(f"{Colors.OKBLUE}ℹ {msg}{Colors.ENDC}")


def print_warning(msg):
    print(f"{Colors.WARNING}⚠ {msg}{Colors.ENDC}")


def print_error(msg):
    print(f"{Colors.FAIL}✗ {msg}{Colors.ENDC}")


def run_command(cmd, check=True, capture_output=True):
    """Run a shell command and return result"""
    try:
        result = subprocess.run(
            cmd,
            shell=True,
            check=check,
            capture_output=capture_output,
            text=True
        )
        return result
    except subprocess.CalledProcessError as e:
        print_error(f"Command failed: {cmd}")
        print_error(f"Error: {e.stderr}")
        raise


def check_git_clean(force=False):
    """Verify git working tree is clean"""
    print_info("Checking git working tree status...")
    result = run_command("git status --porcelain")
    
    if result.stdout.strip():
        if force:
            print_warning("Working tree has uncommitted changes, but --force flag is set")
            return True
        else:
            print_error("Working tree has uncommitted changes!")
            print_error("Please commit or stash your changes first, or use --force flag")
            print("\nUncommitted changes:")
            print(result.stdout)
            return False
    
    print_success("Working tree is clean")
    return True


def check_remotes():
    """Verify git remotes are configured correctly"""
    print_info("Checking git remotes...")
    
    result = run_command("git remote -v")
    remotes = result.stdout
    
    has_origin = "origin" in remotes and "sandeepbazar/ibm-fusion-mcp-server" in remotes
    has_upstream = "upstream" in remotes and "containers/kubernetes-mcp-server" in remotes
    
    if not has_origin:
        print_error("Remote 'origin' not configured correctly!")
        print_error("Expected: sandeepbazar/ibm-fusion-mcp-server")
        print("\nCurrent remotes:")
        print(remotes)
        return False
    
    if not has_upstream:
        print_warning("Remote 'upstream' not configured!")
        print_info("Adding upstream remote...")
        run_command("git remote add upstream https://github.com/containers/kubernetes-mcp-server.git")
        print_success("Added upstream remote")
    
    print_success("Git remotes configured correctly")
    return True


def sync_upstream():
    """Fetch and merge upstream changes"""
    print_header("SYNCING WITH UPSTREAM")
    
    print_info("Fetching upstream...")
    run_command("git fetch upstream")
    print_success("Fetched upstream")
    
    print_info("Checking out main branch...")
    run_command("git checkout main")
    print_success("On main branch")
    
    print_info("Merging upstream/main...")
    result = run_command("git merge upstream/main", check=False)
    
    if result.returncode != 0:
        print_error("Merge conflicts detected!")
        print_error("Please resolve conflicts manually and run this script again")
        sys.exit(1)
    
    print_success("Merged upstream/main successfully")


def apply_toolsets_patch():
    """Apply Fusion integration hook to pkg/toolsets/toolsets.go"""
    print_info("Applying patch to pkg/toolsets/toolsets.go...")
    
    file_path = Path("pkg/toolsets/toolsets.go")
    if not file_path.exists():
        print_error(f"File not found: {file_path}")
        print_error("Upstream structure may have changed significantly!")
        return False
    
    content = file_path.read_text()
    
    # Check if Fusion hook already exists
    if "registerFusionTools" in content and "SetFusionRegistration" in content:
        print_success("Fusion hooks already present in toolsets.go")
        return True
    
    # Find the end of the file (after last function)
    # We'll add our code before the final closing brace or at the end
    
    fusion_code = '''
func init() {
	// IBM Fusion extension integration point
	// This is the single hook for registering IBM Fusion tools
	// Tools are only registered if FUSION_TOOLS_ENABLED=true
	registerFusionTools()
}

// registerFusionTools is a placeholder that will be implemented by the fusion package
// This allows the fusion package to register itself without modifying upstream code
var registerFusionTools = func() {}

// SetFusionRegistration allows the fusion package to hook into the registration process
// This is the single integration point for IBM Fusion tools
func SetFusionRegistration(fn func()) {
	registerFusionTools = fn
}
'''
    
    # Append at the end of the file
    if not content.endswith('\n'):
        content += '\n'
    
    content += fusion_code
    
    file_path.write_text(content)
    print_success("Applied Fusion hooks to toolsets.go")
    return True


def apply_modules_patch():
    """Apply Fusion import to pkg/mcp/modules.go"""
    print_info("Applying patch to pkg/mcp/modules.go...")
    
    file_path = Path("pkg/mcp/modules.go")
    if not file_path.exists():
        print_error(f"File not found: {file_path}")
        print_error("Upstream structure may have changed significantly!")
        return False
    
    content = file_path.read_text()
    
    # Check if Fusion import already exists
    if "pkg/toolsets/fusion" in content:
        print_success("Fusion import already present in modules.go")
        return True
    
    # Find the import block and add fusion import
    # Look for the pattern: import (\n\t_ "..."
    import_pattern = r'(import\s*\(\s*\n)(\s*_\s*"[^"]+"\s*\n)'
    
    if not re.search(import_pattern, content):
        print_error("Could not find import block in modules.go")
        print_error("File structure may have changed!")
        return False
    
    # Insert fusion import after the first import, maintaining alphabetical order
    # Find position after "config" and before "helm"
    fusion_import = '\t_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/fusion"\n'
    
    # Split by lines and rebuild
    lines = content.split('\n')
    new_lines = []
    import_added = False
    in_import_block = False
    
    for line in lines:
        if 'import (' in line:
            in_import_block = True
            new_lines.append(line)
            continue
        
        if in_import_block and not import_added:
            # Add fusion import after core and before helm
            if 'pkg/toolsets/core' in line:
                new_lines.append(line)
                new_lines.append(fusion_import.rstrip())
                import_added = True
                continue
            elif 'pkg/toolsets/helm' in line and not import_added:
                new_lines.append(fusion_import.rstrip())
                new_lines.append(line)
                import_added = True
                continue
        
        if ')' in line and in_import_block:
            in_import_block = False
        
        new_lines.append(line)
    
    content = '\n'.join(new_lines)
    file_path.write_text(content)
    print_success("Applied Fusion import to modules.go")
    return True


def run_tests():
    """Run go tests"""
    print_header("RUNNING TESTS")
    
    print_info("Running: go test ./...")
    result = run_command("go test ./...", check=False, capture_output=False)
    
    if result.returncode != 0:
        print_error("Tests failed!")
        print_error("Please fix test failures before proceeding")
        return False
    
    print_success("All tests passed")
    return True


def commit_and_push():
    """Commit and push changes if any"""
    print_header("COMMITTING CHANGES")
    
    result = run_command("git status --porcelain")
    
    if not result.stdout.strip():
        print_info("No changes to commit")
        return True
    
    print_info("Changes detected:")
    print(result.stdout)
    
    print_info("Adding changes...")
    run_command("git add -A")
    
    print_info("Committing changes...")
    commit_msg = "chore(fusion): sync upstream and apply fusion integration hooks"
    run_command(f'git commit -m "{commit_msg}"')
    print_success("Changes committed")
    
    print_info("Pushing to origin main...")
    result = run_command("git push origin main", check=False)
    
    if result.returncode != 0:
        print_error("Push failed!")
        print_error("You may need to pull first or resolve conflicts")
        return False
    
    print_success("Pushed to origin main")
    return True


def main():
    """Main execution flow"""
    print_header("IBM FUSION MCP SERVER - UPSTREAM SYNC")
    print(f"Maintainer: Sandeep Bazar (GitHub: sandeepbazar)\n")
    
    # Parse arguments
    force = "--force" in sys.argv
    
    # Change to repo root
    repo_root = Path(__file__).parent.parent.parent
    os.chdir(repo_root)
    print_info(f"Working directory: {repo_root}")
    
    # Step 1: Check git status
    if not check_git_clean(force):
        sys.exit(1)
    
    # Step 2: Check remotes
    if not check_remotes():
        sys.exit(1)
    
    # Step 3: Sync upstream
    sync_upstream()
    
    # Step 4: Apply patches
    print_header("APPLYING FUSION PATCHES")
    
    if not apply_toolsets_patch():
        sys.exit(1)
    
    if not apply_modules_patch():
        sys.exit(1)
    
    print_success("All patches applied successfully")
    
    # Step 5: Run tests
    if not run_tests():
        print_warning("Tests failed, but patches were applied")
        print_warning("Please fix tests manually")
        sys.exit(1)
    
    # Step 6: Commit and push
    if not commit_and_push():
        print_warning("Failed to push changes")
        print_warning("Changes are committed locally, please push manually")
        sys.exit(1)
    
    # Success summary
    print_header("SYNC COMPLETE")
    print_success("✓ Synced with upstream")
    print_success("✓ Applied Fusion integration hooks")
    print_success("✓ Tests passed")
    print_success("✓ Changes committed and pushed")
    print(f"\n{Colors.OKGREEN}{Colors.BOLD}All done! 🎉{Colors.ENDC}\n")


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print_error("\n\nInterrupted by user")
        sys.exit(1)
    except Exception as e:
        print_error(f"\n\nUnexpected error: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)

# Made with Bob
