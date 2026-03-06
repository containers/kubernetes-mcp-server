# Tekton Task Stack

Tekton-focused MCP eval tasks live here. Each folder represents a self-contained scenario that exercises the Tekton toolset (Pipeline and PipelineRun management, Task and TaskRun lifecycle).

All tasks use the `tekton-eval` namespace and require Tekton Pipelines to be installed in the cluster (`tekton.dev/v1` CRDs must be available).

## Tasks Defined

### Pipeline Operations

- **[easy] list-pipelines** – List Tekton Pipelines in a namespace
  - **Prompt:** *List all Tekton Pipelines in the tekton-eval namespace.*
  - **Tests:** `tekton_pipeline_list` tool

- **[easy] get-pipeline** – Retrieve a specific Pipeline by name
  - **Prompt:** *Get the Tekton Pipeline named hello-pipeline in the tekton-eval namespace.*
  - **Tests:** `tekton_pipeline_get` tool

- **[easy] create-pipeline** – Create a new Pipeline from a YAML definition
  - **Prompt:** *Create a Tekton Pipeline named "greet-pipeline" in the tekton-eval namespace with a single task step that references a Task named "greet-task".*
  - **Tests:** `tekton_pipeline_create` tool

- **[medium] start-pipeline** – Start a Pipeline by triggering a new PipelineRun
  - **Prompt:** *Start the Tekton Pipeline named hello-pipeline in the tekton-eval namespace.*
  - **Tests:** `tekton_pipeline_start` tool (creates a PipelineRun for the Pipeline)

### PipelineRun Operations

- **[easy] list-pipelineruns** – List PipelineRuns in a namespace
  - **Prompt:** *List all Tekton PipelineRuns in the tekton-eval namespace.*
  - **Tests:** `tekton_pipelinerun_list` tool

- **[medium] delete-pipelinerun** – Delete a specific PipelineRun
  - **Prompt:** *Delete the Tekton PipelineRun named old-run in the tekton-eval namespace.*
  - **Tests:** `tekton_pipelinerun_delete` tool

- **[medium] restart-pipelinerun** – Restart a PipelineRun by creating a new one with the same spec
  - **Prompt:** *Restart the Tekton PipelineRun named test-run in the tekton-eval namespace.*
  - **Tests:** `tekton_pipelinerun_restart` tool (creates a new PipelineRun with the same spec)

### Task Operations

- **[easy] create-task** – Create a new Tekton Task from a YAML definition
  - **Prompt:** *Create a Tekton Task named "echo-task" in the tekton-eval namespace with a single step that echoes "Hello, Tekton!".*
  - **Tests:** `tekton_task_create` tool

- **[medium] start-task** – Start a Task by creating a TaskRun for it
  - **Prompt:** *Start the Tekton Task named echo-task in the tekton-eval namespace.*
  - **Tests:** `tekton_task_start` tool (creates a TaskRun referencing the Task)

## Prerequisites

Tekton Pipelines must be installed in the cluster. You can install the latest release with:

```shell
kubectl apply --filename https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml
```

Verify the installation:

```shell
kubectl get pods -n tekton-pipelines
```

## Adding a New Task

1. Create a new subdirectory (e.g., `update-pipeline/`) with a `task.yaml` following the `mcpchecker/v1alpha2` format.
2. Set `metadata.labels.suite: tekton` so the task is grouped correctly in eval reports.
3. Use `tekton-eval` as the namespace for consistency across the task stack.
4. Include `ignoreNotFound: true` on all cleanup steps to keep tasks idempotent.
5. For verify steps that check Kubernetes resource state use `script.inline` with `kubectl`; for verifying agent output use `llmJudge`.
