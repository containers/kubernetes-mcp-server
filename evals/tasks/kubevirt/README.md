# KubeVirt Task Stack

KubeVirt-focused MCP tasks live here. Each folder under this directory represents a self-contained scenario that exercises the KubeVirt toolset (virtual machine creation, lifecycle management, troubleshooting).

## Adding a New Task

1. Create a new subdirectory (e.g., `create-vm-foo/`) and place the scenario YAML plus any helper scripts or artifacts inside it.
2. Make sure the YAML's `metadata` block includes `name` and `difficulty` so it shows up correctly in the catalog below.
3. Keep prompts concise and action-oriented; verification commands should rely on KubeVirt resources and helper functions whenever possible.

## Tasks Defined

### VM Creation

- **[easy] create-vm-basic** - Create a basic Fedora virtual machine
  - **Prompt:** *Please create a Fedora virtual machine named test-vm in the vm-test namespace.*

- **[easy] create-vm-ubuntu** - Create an Ubuntu virtual machine
  - **Prompt:** *Create an Ubuntu virtual machine named ubuntu-vm in the vm-test namespace.*

- **[easy] create-vm-with-instancetype** - Create a VM using VirtualMachineInstancetype
  - **Prompt:** *Create a Fedora virtual machine with specific instance types and preferences.*

- **[easy] create-vm-with-size** - Create a VM with specific size requirements
  - **Prompt:** *Create a virtual machine with custom CPU and memory specifications.*

### VM Lifecycle Management

- **[medium] pause-vm** - Pause a running virtual machine
  - **Prompt:** *Please pause the virtual machine named paused-vm in the vm-test namespace.*

- **[medium] delete-vm** - Delete a virtual machine
  - **Prompt:** *Please delete the virtual machine named deleted-vm in the vm-test namespace.*

### VM Modification

- **[hard] update-vm-resources** - Update VM CPU and memory resources
  - **Prompt:** *A VirtualMachine named test-vm-update exists in the vm-test namespace. It currently has 1 vCPU and 2Gi of memory. Please update the VirtualMachine to add an additional vCPU (making it 2 vCPUs total) and increase the memory to at least 3Gi.*

## Helper Scripts

Many tasks rely on helper scripts located in `evals/tasks/kubevirt/helpers/`:

- `verify-vm.sh` - Common VM verification functions used across multiple test scenarios

## Running Tasks

These tasks are designed to be used with the gevals evaluation framework. Each task includes:

- **setup** - Prepares the test environment (creates namespace, sets up initial VM state)
- **verify** - Validates the expected outcome after the agent completes the task
- **cleanup** - Removes resources created during the test
- **prompt** - The instruction given to the AI agent

Example workflow:

1. Setup creates the initial state
2. Agent receives the prompt and executes actions using MCP tools
3. Verify checks if the agent accomplished the goal
4. Cleanup removes test resources
