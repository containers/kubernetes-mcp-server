## KubeVirt integration

This server can expose KubeVirt tools so assistants can create and manage virtual machines running on Kubernetes clusters with [KubeVirt](https://kubevirt.io/).

### Prerequisites

- A Kubernetes cluster with [KubeVirt](https://kubevirt.io/user-guide/cluster_admin/installation/) installed
- The KubeVirt API resources (`VirtualMachine`, `VirtualMachineInstance`, `VirtualMachineClone`, etc.) must be available in the cluster

### Enable the KubeVirt toolset

The KubeVirt toolset is not enabled by default. Enable it via the CLI flag or a TOML configuration file.

CLI:

```shell
kubernetes-mcp-server --toolsets core,kubevirt
```

Config (TOML):

```toml
toolsets = ["core", "kubevirt"]
```

No additional toolset-specific configuration is required. The server uses your existing Kubernetes credentials (from kubeconfig or in-cluster) to interact with the KubeVirt API.

### Available tools

#### `vm_create`

Create a VirtualMachine in the cluster. The tool automatically resolves instance types, preferences, and container disk images based on the provided parameters.

- **Workload resolution** - Accepts OS names such as `fedora`, `ubuntu`, `centos`, `centos-stream`, `debian`, `rhel8`, `rhel9`, `rhel10`, `opensuse`, `opensuse-tumbleweed`, and `opensuse-leap`. These are resolved to container disk images from `quay.io/containerdisks`. Full container disk image URLs are also accepted. If DataSources are available in the cluster, the tool will match against those first.
- **Instance type resolution** - Automatically selects an appropriate instance type based on the `size` (e.g., `small`, `medium`, `large`) and `performance` (e.g., `general-purpose`, `overcommitted`, `compute-optimized`, `memory-optimized`) hints. Instance types can also be specified explicitly.
- **Preference resolution** - Resolves VM preferences from cluster resources or DataSource defaults.
- **Networking** - Supports attaching secondary network interfaces via Multus NetworkAttachmentDefinitions.
- **Run strategy** - VMs are created in `Halted` state by default. Set `autostart` to `true` to start the VM immediately.

#### `vm_lifecycle`

Manage the lifecycle of an existing VirtualMachine:

- `start` - Sets the runStrategy to `Always`, starting the VM.
- `stop` - Sets the runStrategy to `Halted`, stopping the VM.
- `restart` - Stops and then starts the VM.

#### `vm_clone`

Clone an existing VirtualMachine by creating a `VirtualMachineClone` resource. This creates a copy of the source VM with a new name using the KubeVirt Clone API.

### Available prompts

#### `vm-troubleshoot`

Generate a step-by-step troubleshooting guide for diagnosing VirtualMachine issues. The prompt collects and presents:

1. VirtualMachine status
2. VirtualMachineInstance status
3. VM volumes configuration
4. virt-launcher Pod information
5. virt-launcher Pod logs
6. Related Kubernetes events

Usage requires the `namespace` and `name` of the VirtualMachine to troubleshoot.

#### `windows-golden-image`

Create a Windows golden image by running the `windows-efi-installer` Tekton pipeline
from the [KubeVirt Tekton Pipelines catalog](https://artifacthub.io/packages/tekton-pipeline/kubevirt-tekton-pipelines/windows-efi-installer).
The pipeline downloads a Windows ISO, modifies it for EFI automated installation,
creates a VM, installs Windows, and produces a bootable DataSource/DataVolume suitable
as a golden image for Windows VirtualMachines.

**How it works:**
The prompt dynamically fetches the latest PipelineRun template from Artifact Hub, ensuring
compatibility with the specified pipeline version. This means the generated YAML will always
match the format expected by the pipeline, even when new versions introduce changes.

**Prerequisites:**
- KubeVirt
- Tekton Pipelines
- Both `kubevirt` and `tekton` toolsets must be enabled

**Arguments:**
- `winImageDownloadURL` (required): Microsoft Windows ISO download URL
- `namespace` (optional): Target namespace for the PipelineRun
- `windowsVersion` (optional): Windows version — `10`, `11`, `2k22` (default), or `2k25`
- `pipelineVersion` (optional): Pipeline version to use (default: latest). Specify a version like `0.25.0` if needed

**EULA Notice:**
The prompt instructs the AI agent to ask for explicit user acceptance of the Microsoft
EULA before creating the PipelineRun. The `acceptEula` parameter is only set to `true`
after the user confirms agreement.

**Usage:**
Requires both `kubevirt` and `tekton` toolsets enabled:
```shell
kubernetes-mcp-server --toolsets core,kubevirt,tekton
```
