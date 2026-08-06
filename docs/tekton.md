# Tekton Toolset

The `tekton` toolset adds Tekton-specific helpers on top of the generic Kubernetes resource tools.

Enable it with:

```shell
kubernetes-mcp-server --toolsets core,config,tekton
```

## PipelineRun operations

- `tekton_pipeline_start` starts a Pipeline by creating a PipelineRun.
- `tekton_pipelinerun_lifecycle` restarts a PipelineRun from its existing spec or cancels it by setting `spec.status` to `Cancelled`.
- `tekton_pipelinerun_logs` collects logs from TaskRuns owned by a PipelineRun. Use the optional `task` and `step` parameters to narrow the output.

## TaskRun operations

- `tekton_task_start` starts a Task by creating a TaskRun.
- `tekton_taskrun_restart` creates a new TaskRun from an existing TaskRun spec.
- `tekton_taskrun_logs` resolves a TaskRun pod and returns step/sidecar logs. Use the optional `step` parameter to return one step only.

Pipeline-as-Code `Repository` and operator `TektonConfig` resources are ordinary Kubernetes resources; use `resources_list` and `resources_get` for those.

List Pipeline-as-Code repositories in a namespace with `resources_list`:

```json
{"apiVersion":"pipelinesascode.tekton.dev/v1alpha1","kind":"Repository","namespace":"my-namespace"}
```

Get the usual cluster `TektonConfig` with `resources_get`:

```json
{"apiVersion":"operator.tekton.dev/v1alpha1","kind":"TektonConfig","name":"config"}
```

These tools are read-only except the start and lifecycle operations.

## Troubleshooting prompt

Use the `pipeline-troubleshoot` prompt with `namespace` and `name` to gather PipelineRun status, the exact resolved or embedded PipelineSpec when available, the referenced Pipeline otherwise, related TaskRuns, failed or errored step logs, warning events, Pipeline-as-Code repositories, and TektonConfig context into one diagnostic prompt. Missing definitions are reported without preventing the remaining data from being collected. The guide structures the response as PipelineRun Status, TaskRun Status, Failed Step Logs, Events, Troubleshooting Analysis, and Fix Suggestions.
