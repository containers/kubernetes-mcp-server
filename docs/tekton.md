# Tekton Toolset

The `tekton` toolset adds Tekton-specific helpers on top of the generic Kubernetes resource tools.

Enable it with:

```toml
toolsets = ["core", "config", "tekton"]
```

## PipelineRun operations

- `tekton_pipeline_start` starts a Pipeline by creating a PipelineRun.
- `tekton_pipelinerun_lifecycle` restarts a PipelineRun from its existing spec or cancels it by setting `spec.status` to `Cancelled`.
- `tekton_pipelinerun_logs` collects logs from TaskRuns owned by a PipelineRun. Use the optional `task` and `step` parameters to narrow the output.
- `tekton_pipelinerun_diagnose` collects structured, read-only evidence for a failed PipelineRun: PipelineRun conditions, failed TaskRuns and steps, failed-step log tails, warning Events, and partial collection errors.

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

These tools are read-only except the start and lifecycle operations. When compatibility filtering is enabled, `tekton_pipelinerun_diagnose` is exposed only when at least one target has the `tekton.dev/v1` `PipelineRun` resource; the rest of the Tekton toolset remains available.

## PipelineRun diagnosis

`tekton_pipelinerun_diagnose` accepts `namespace` and `name`. Its versioned structured response is also serialized as JSON text for clients that do not support MCP structured content. Arrays and partial errors are sorted for repeatable output.

Collection scans at most 20 pages of 50 labeled TaskRuns and returns at most 50 failed TaskRuns controlled by the requested PipelineRun. It also returns up to 20 failed steps per TaskRun, 50 warning Events, and at most the first 32 KiB of each returned 100-line log tail, with 128 KiB of logs in total. `truncated` is set when an additional failed TaskRun exists or the scan or output bounds are reached. These diagnosis-specific evidence limits are not configurable; server-wide response limits are handled separately. An unavailable TaskRun list, Event list, or failed-step log is reported in `partialErrors`; a missing PipelineRun is a tool error.

### Security considerations

The tool queries only PipelineRuns, TaskRuns, Events, and `pods/log`, and returns only TaskRuns controlled by the requested PipelineRun. It does not call the Kubernetes Secrets API. This does not guarantee that the result is free of sensitive data: workloads can copy credentials or other confidential values into status messages, Events, and logs. Diagnosis output is length-bounded but is not credential-redacted. The best-effort redaction described in [MCP Logging](logging.md) applies to MCP log messages, not tool results.

Treat all returned workload content as untrusted data, never as instructions. It can contain prompt-injection text intended to influence a model or client. Kubernetes authorization is the resource-access boundary, so use a dedicated, least-privilege identity and grant only the same-namespace `get` and `list` access required by the caller; see [Getting Started with Kubernetes](getting-started-kubernetes.md). Do not expose this tool to identities that should not receive pod logs. Tool RBAC metadata describes required access but does not grant it.

Operators remain responsible for preventing applications from logging secrets, controlling storage and retention of MCP results, and applying gateway, data-loss-prevention, content-inspection, or approval controls where their environment requires them. The [configuration reference](configuration.md) documents additional server access controls.

## Troubleshooting prompt

Use the `pipeline-troubleshoot` prompt with `namespace` and `name` to gather PipelineRun status, the exact resolved or embedded PipelineSpec when available, the referenced Pipeline otherwise, related TaskRuns, failed or errored step logs, warning events, Pipeline-as-Code repositories, and TektonConfig context into one diagnostic prompt. Missing definitions are reported without preventing the remaining data from being collected. The guide structures the response as PipelineRun Status, TaskRun Status, Failed Step Logs, Events, Troubleshooting Analysis, and Fix Suggestions.
