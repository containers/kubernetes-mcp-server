# Tekton Task Stack

Tekton-focused MCP eval tasks live here. Each folder represents a self-contained scenario that exercises the Tekton toolset (Pipeline and PipelineRun management, Task and TaskRun lifecycle).

All tasks use the `tekton-eval` namespace and require Tekton Pipelines to be installed in the cluster (`tekton.dev/v1` CRDs must be available).

## Tasks Defined

### Pipeline Operations

- **[easy] list-pipelines** – List Tekton Pipelines in a namespace
  - **Prompt:** *List all Tekton Pipelines in the tekton-eval namespace.*
  - **Tests:** `resources_list` tool (core toolset)

- **[easy] get-pipeline** – Retrieve a specific Pipeline by name
  - **Prompt:** *Get the Tekton Pipeline named hello-pipeline in the tekton-eval namespace.*
  - **Tests:** `resources_get` tool (core toolset)

- **[easy] create-pipeline** – Create a new Pipeline from a YAML definition
  - **Prompt:** *Create a Tekton Pipeline named "greet-pipeline" in the tekton-eval namespace with a single task step that references a Task named "greet-task".*
  - **Tests:** `resources_create` tool (core toolset)

- **[medium] start-pipeline** – Start a Pipeline by triggering a new PipelineRun
  - **Prompt:** *Start the Tekton Pipeline named hello-pipeline in the tekton-eval namespace.*
  - **Tests:** `resources_create` tool (core toolset)

### PipelineRun Operations

- **[easy] list-pipelineruns** – List PipelineRuns in a namespace
  - **Prompt:** *List all Tekton PipelineRuns in the tekton-eval namespace.*
  - **Tests:** `resources_list` tool (core toolset)

- **[medium] delete-pipelinerun** – Delete a specific PipelineRun
  - **Prompt:** *Delete the Tekton PipelineRun named old-run in the tekton-eval namespace.*
  - **Tests:** `resources_delete` tool (core toolset)

- **[medium] restart-pipelinerun** – Restart a PipelineRun by creating a new one with the same spec
  - **Prompt:** *Restart the Tekton PipelineRun named test-run in the tekton-eval namespace.*
  - **Tests:** `resources_get` + `resources_create` tools (core toolset)

### Task Operations

- **[easy] create-task** – Create a new Tekton Task from a YAML definition
  - **Prompt:** *Create a Tekton Task named "echo-task" in the tekton-eval namespace with a single step that echoes "Hello, Tekton!".*
  - **Tests:** `resources_create` tool (core toolset)

- **[medium] start-task** – Start a Task by creating a TaskRun for it
  - **Prompt:** *Start the Tekton Task named echo-task in the tekton-eval namespace.*
  - **Tests:** `resources_create` tool (core toolset)

## Prerequisites

Tekton Pipelines must be installed in the cluster. Install it with:

```shell
make tekton-install
```

Verify the installation:

```shell
make tekton-status
```

## Adding a New Task

1. Create a new subdirectory (e.g., `update-pipeline/`) with a `task.yaml` following the `mcpchecker/v1alpha2` format.
2. Set `metadata.labels.suite: tekton` and `metadata.labels.requires: tekton` so the task is grouped correctly in eval reports and the eval framework can filter tasks that need Tekton installed.
3. Use `tekton-eval` as the namespace for consistency across the task stack.
4. Use the shared `helpers/cleanup-namespace.sh` script for namespace cleanup in setup steps.
5. Include `ignoreNotFound: true` on all cleanup steps to keep tasks idempotent.
6. For verify steps that check Kubernetes resource state use `script.inline` with `kubectl`; for verifying agent output use `llmJudge`.
