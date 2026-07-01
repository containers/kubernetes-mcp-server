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

- **[hard] update-vm-instancetype** - Double a VM's memory by switching its instancetype
  - **Prompt:** *A Fedora VirtualMachine named fedora-vm exists in the vm-test-instancetype-update namespace using the u1.small instancetype. Double its memory by updating the instance type.*

### VM Troubleshooting

- **[hard] troubleshoot-vm** - Use the vm-troubleshoot prompt to diagnose VirtualMachine issues
  - **Prompt:** *There is a VirtualMachine named "broken-vm" in the vm-test namespace that is not working correctly. Please use the vm-troubleshoot prompt to diagnose the issue with this VirtualMachine. Follow the troubleshooting guide and report your findings, including the root cause and recommended action.*
  - **Tests:** Agent's ability to use MCP prompts for guided troubleshooting workflows

### HCO (HyperConverged Cluster Operator)

These tasks evaluate the agent's ability to work with the HyperConverged Cluster Operator, which manages KubeVirt, CDI, and other components as a single operator. They test whether the agent can leverage the `hco-status` prompt and other tools to diagnose and report on HCO state.

**Prerequisites:** HCO must be deployed — use `make local-env-setup-hco` to set up a Kind cluster with HCO.

- **[easy] hco-get-status** - Retrieve and interpret the HyperConverged CR status and conditions
  - **Prompt:** *Describe the current status of HCO and whether the cluster is healthy.*
  - **Tests:** Can the agent retrieve and interpret HCO conditions?

- **[easy] hco-check-version** - Determine the installed HCO version and managed component versions
  - **Prompt:** *What version of HCO is installed, and what versions of its managed components are running?*
  - **Tests:** Can the agent extract version info from the HCO CR status?

- **[easy] hco-get-feature-gates** - List the feature gates configured in the HyperConverged CR
  - **Prompt:** *What feature gates are configured in HCO, and which ones are currently enabled?*
  - **Tests:** Can the agent navigate the HCO spec structure to find feature gate configuration?

- **[medium] hco-check-component-health** - Check health of individual HCO-managed components
  - **Prompt:** *Check the health of each component managed by HCO and report any issues.*
  - **Tests:** Can the agent correlate HCO with individual component CRs?

- **[medium] hco-get-live-migration-config** - Retrieve and explain the live migration configuration
  - **Prompt:** *What are the current live migration settings in HCO? Report each setting, explain what it controls, and whether the value is custom or default.*
  - **Tests:** Can the agent find and interpret the liveMigrationConfig section?

- **[medium] hco-list-managed-components** - List all components managed by HCO with their status
  - **Prompt:** *List all components managed by HCO, their versions, and whether each one is healthy.*
  - **Tests:** Can the agent discover the relationship between HCO and its managed operators?

- **[medium] hco-status-prompt** - Use the hco-status prompt to generate a full status report
  - **Prompt:** *Use the hco-status prompt to generate a status report for the HyperConverged Cluster Operator. Analyze the report and summarize the overall health, managed component status, and any warnings.*
  - **Tests:** Can the agent use the `hco-status` MCP prompt to produce and interpret a status report?

- **[hard] hco-diagnose-degraded** - Diagnose why HCO is reporting an unusual condition
  - **Prompt:** *HCO seems to be reporting an unusual condition. Investigate the issue, determine the root cause, and recommend remediation steps.*
  - **Tests:** Can the agent correlate HCO conditions with underlying component states to diagnose issues?

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
