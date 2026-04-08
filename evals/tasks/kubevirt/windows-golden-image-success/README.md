# Windows Golden Image Evaluation Tests

Two evaluation tasks for the `windows-golden-image` prompt.

## Tasks

### `windows-golden-image-success/task.yaml` (Hard)

The agent is asked to invoke the prompt, accept the EULA, and create a PipelineRun.

The verify script fails if no PipelineRun is created. When one is found, it checks:
- `acceptEula` is set to `true`
- `pipelineRef.resolver` is `hub`
- `pipelineRef` references the `windows-efi-installer` pipeline from the `kubevirt-tekton-pipelines` catalog
- `pipelineRef` version matches the `0.xx.0` format

### `windows-golden-image-no-eula/task.yaml` (Medium)

The agent is asked to review the prompt output only — **not** to accept the EULA or create any resources.

The verify script fails if a PipelineRun is found in the namespace, meaning the agent disobeyed the instructions.

## Prerequisites

- Kubernetes cluster with KubeVirt and Tekton Pipelines installed
- Both `kubevirt` and `tekton` toolsets enabled in the MCP server

## Running the Tests

```bash
mcpchecker check evals/tasks/kubevirt/windows-golden-image-success/task.yaml
mcpchecker check evals/tasks/kubevirt/windows-golden-image-no-eula/task.yaml
```

## Notes

- Tests use placeholder ISO URLs (`https://example.com/...`) — no actual Windows download occurs
- The pipeline is not executed; only the PipelineRun structure is verified
