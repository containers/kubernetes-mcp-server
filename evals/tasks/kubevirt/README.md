# KubeVirt Task Stack

KubeVirt-focused MCP tasks live here. Each folder under this directory represents a self-contained scenario that exercises the KubeVirt toolset (virtual machine creation, lifecycle management, troubleshooting).

## Adding a New Task

1. Create a new subdirectory (e.g., `create-vm-foo/`) and place the scenario YAML plus any helper scripts or artifacts inside it.
2. Make sure the YAML's `metadata` block includes `name` and `difficulty` so it shows up correctly in the catalog below.
3. Declare the kubernetes extension in `spec.requires` and use `k8s.create`/`k8s.delete`/`k8s.wait` for resource management where possible.
4. Keep prompts concise and action-oriented; verification commands should rely on KubeVirt resources and helper functions whenever possible.

## Tasks Defined

### VM Creation

- **[easy] create-vm-basic** - Create a basic Fedora virtual machine
  - **Prompt:** *Create a Fedora virtual machine named test-vm in the vm-test namespace.*

- **[easy] create-vm-ubuntu** - Create an Ubuntu virtual machine
  - **Prompt:** *Create an Ubuntu virtual machine named ubuntu-vm in the vm-test namespace.*

- **[easy] create-vm-with-instancetype** - Create a VM using VirtualMachineInstancetype
  - **Prompt:** *Create a Fedora virtual machine with specific instance types and preferences.*

- **[easy] create-vm-with-size** - Create a VM with specific size requirements
  - **Prompt:** *Create a virtual machine with custom CPU and memory specifications.*

- **[hard] create-vm-with-vlan** - Create a VM with a Multus secondary network interface
  - **Prompt:** *Please create a Fedora virtual machine named test-vm in the vm-test namespace with a secondary network interface connected to the vlan-network multus network.*

### VM Lifecycle Management

- **[medium] pause-vm** - Pause a running virtual machine
  - **Prompt:** *Please pause the virtual machine named paused-vm in the vm-test namespace.*

- **[medium] delete-vm** - Delete a virtual machine
  - **Prompt:** *Please delete the virtual machine named deleted-vm in the vm-test namespace.*

### VM Snapshots

- **[medium] snapshot-vm** - Create a snapshot of a virtual machine
  - **Prompt:** *Create a snapshot named test-snapshot of the virtual machine snapshot-test-vm in the vm-test namespace.*

- **[medium] restore-vm** - Restore a virtual machine from a snapshot
  - **Prompt:** *Restore the snapshot named restore-snapshot to a new virtual machine named restored-vm.*

### VM Modification

- **[hard] update-vm-resources** - Update VM CPU and memory resources
  - **Prompt:** *A VirtualMachine named test-vm-update exists in the vm-test namespace. It currently has 1 vCPU and 2Gi of memory. Please update the VirtualMachine to add an additional vCPU (making it 2 vCPUs total) and increase the memory to at least 3Gi.*

### VM Troubleshooting

- **[hard] troubleshoot-vm** - Use the vm-troubleshoot prompt to diagnose VirtualMachine issues
  - **Prompt:** *There is a VirtualMachine named "broken-vm" in the vm-test namespace that is not working correctly. Please use the vm-troubleshoot prompt to diagnose the issue with this VirtualMachine. Follow the troubleshooting guide and report your findings, including the root cause and recommended action.*
  - **Tests:** Agent's ability to use MCP prompts for guided troubleshooting workflows

## Helper Scripts

Some verification steps rely on helper scripts located in `helpers/`:

- `verify-vm.sh` - Common VM verification functions used across multiple test scenarios

## Running Tasks

These tasks are designed to be used with the [mcpchecker](https://github.com/mcpchecker/mcpchecker) evaluation framework. Each task includes:

- **setup** - Array of steps that prepare the test environment (creates namespace, sets up initial VM state)
- **verify** - Array of steps that validate the expected outcome after the agent completes the task
- **cleanup** - Array of steps that remove resources created during the test
- **prompt** - The instruction given to the AI agent

Example workflow:

1. Setup steps create the initial state (using `k8s.create` and optional script steps)
2. Agent receives the prompt and executes actions using MCP tools
3. Verify steps check if the agent accomplished the goal (using `k8s.wait` or script steps)
4. Cleanup steps remove test resources (using `k8s.delete`)
