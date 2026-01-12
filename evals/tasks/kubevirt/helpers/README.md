# Test Verification Helpers

This directory contains shared helper functions for VirtualMachine test verification.

## Usage

Source the helper script in your test verification section:

```bash
#!/usr/bin/env bash
source "$(dirname "${BASH_SOURCE[0]}")/../../helpers/verify-vm.sh"

# Use helper functions
verify_vm_exists "test-vm" "vm-test" || exit 1
verify_container_disk "test-vm" "vm-test" "fedora" || exit 1
verify_run_strategy "test-vm" "vm-test" || exit 1
verify_no_deprecated_running_field "test-vm" "vm-test" || exit 1
```

## Available Functions

### verify_vm_exists
Waits for a VirtualMachine to be created.

**Usage:** `verify_vm_exists <vm-name> <namespace> [timeout]`

**Example:**
```bash
verify_vm_exists "my-vm" "vm-test" "30s" || exit 1
```

**Default timeout:** 30s

---

### verify_container_disk
Verifies that a VM uses a specific container disk OS (checks all volumes).

**Usage:** `verify_container_disk <vm-name> <namespace> <os-name>`

**Example:**
```bash
verify_container_disk "my-vm" "vm-test" "fedora" || exit 1
verify_container_disk "ubuntu-vm" "vm-test" "ubuntu" || exit 1
```

---

### verify_run_strategy
Verifies that runStrategy is set (checks both spec and status).

**Usage:** `verify_run_strategy <vm-name> <namespace>`

**Example:**
```bash
verify_run_strategy "my-vm" "vm-test" || exit 1
```

**Note:** This function accepts runStrategy in either `spec.runStrategy` or `status.runStrategy` to accommodate VMs created with the deprecated `running` field.

---

### verify_no_deprecated_running_field
Verifies that the deprecated `running` field is NOT set in the VirtualMachine spec.

**Usage:** `verify_no_deprecated_running_field <vm-name> <namespace>`

**Example:**
```bash
verify_no_deprecated_running_field "my-vm" "vm-test" || exit 1
```

**Note:** The `running` field is deprecated in KubeVirt. VirtualMachines should use `runStrategy` instead. This function ensures compliance with current best practices.

---

### verify_instancetype
Verifies that a VM has an instancetype reference with optional exact match.

**Usage:** `verify_instancetype <vm-name> <namespace> [expected-instancetype] [expected-kind]`

**Examples:**
```bash
# Just verify instancetype exists
verify_instancetype "my-vm" "vm-test" || exit 1

# Verify specific instancetype
verify_instancetype "my-vm" "vm-test" "u1.medium" || exit 1

# Verify instancetype and kind
verify_instancetype "my-vm" "vm-test" "u1.medium" "VirtualMachineClusterInstancetype" || exit 1
```

**Default kind:** VirtualMachineClusterInstancetype

---

### verify_instancetype_contains
Verifies that instancetype name contains a substring (e.g., size like "large").

**Usage:** `verify_instancetype_contains <vm-name> <namespace> <substring> [description]`

**Example:**
```bash
verify_instancetype_contains "my-vm" "vm-test" "large" "requested size 'large'"
verify_instancetype_contains "my-vm" "vm-test" "medium"
```

**Note:** Returns success even if substring not found (prints warning only).

---

### verify_instancetype_prefix
Verifies that instancetype starts with a specific prefix (e.g., performance family like "c1").

**Usage:** `verify_instancetype_prefix <vm-name> <namespace> <prefix> [description]`

**Example:**
```bash
verify_instancetype_prefix "my-vm" "vm-test" "c1" "compute-optimized"
verify_instancetype_prefix "my-vm" "vm-test" "u1" "general-purpose"
```

**Note:** Returns success even if prefix doesn't match (prints warning only).

---

### verify_no_direct_resources
Verifies that VM uses instancetype for resources (no direct memory specification).

**Usage:** `verify_no_direct_resources <vm-name> <namespace>`

**Example:**
```bash
verify_no_direct_resources "my-vm" "vm-test"
```

**Note:** Returns success even if direct resources found (prints warning only).

---

### verify_has_resources_or_instancetype
Verifies that VM has either an instancetype or direct resource specification.

**Usage:** `verify_has_resources_or_instancetype <vm-name> <namespace>`

**Example:**
```bash
verify_has_resources_or_instancetype "my-vm" "vm-test" || exit 1
```

**Note:** Fails only if neither instancetype nor direct resources are present.

---

### verify_cpu_cores
Verifies that a VM has the expected number of CPU cores.

**Usage:** `verify_cpu_cores <vm-name> <namespace> <expected-cores>`

**Example:**
```bash
verify_cpu_cores "my-vm" "vm-test" 2 || exit 1
verify_cpu_cores "my-vm" "vm-test" 4 || exit 1
```

**Note:** This function checks the direct CPU specification in `spec.template.spec.domain.cpu.cores`. For VMs using instancetypes, CPU is defined by the instancetype itself.

---

### verify_memory_increased
Verifies that VM memory is greater than the original value.

**Usage:** `verify_memory_increased <vm-name> <namespace> <original-memory>`

**Examples:**
```bash
verify_memory_increased "my-vm" "vm-test" "2Gi" || exit 1
verify_memory_increased "my-vm" "vm-test" "4096Mi" || exit 1
```

**Note:** This function compares memory values in bytes, supporting Gi, Mi, Ki, G, M, K suffixes. It fails if current memory is not greater than the original value.

## Design Principles

1. **Flexible matching**: Functions use pattern matching instead of exact volume names to handle different VM creation approaches.

2. **Clear output**: Each function prints clear success (✓) or failure (✗) messages.

3. **Warning vs Error**: Some functions print warnings (⚠) for non-critical mismatches but still return success.

4. **Return codes**: Functions return 0 for success, 1 for failure. Always check return codes with `|| exit 1` for critical validations.

## Example Test Verification

```bash
#!/usr/bin/env bash
source "$(dirname "${BASH_SOURCE[0]}")/../../helpers/verify-vm.sh"

# Wait for VM to exist
verify_vm_exists "test-vm" "vm-test" || exit 1

# Verify container disk
verify_container_disk "test-vm" "vm-test" "fedora" || exit 1

# Verify runStrategy is used (not deprecated 'running' field)
verify_run_strategy "test-vm" "vm-test" || exit 1
verify_no_deprecated_running_field "test-vm" "vm-test" || exit 1

# Verify instancetype with size
verify_instancetype "test-vm" "vm-test" || exit 1
verify_instancetype_contains "test-vm" "vm-test" "large"

# Verify no direct resources
verify_no_direct_resources "test-vm" "vm-test"

echo "All validations passed"
exit 0
```
