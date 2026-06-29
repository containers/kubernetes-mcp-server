# MCP Task Library

This directory hosts the reusable task scenarios that power MCP evaluations for the Kubernetes MCP Server. Each task captures a realistic cluster workflow (setup, agent-driven actions, verification, and cleanup) so different agents can be compared against the same benchmark.

## Task Families

- [Core tasks](core/) – core cluster workflows such as creating pods, fixing deployments, managing RBAC, or debugging state issues.
- [Config tasks](config/) – workflows that exercise the configuration toolset (contexts, current config).
- [Helm tasks](helm/) – workflows that exercise the Helm toolset (install, list, uninstall releases).
- [Kiali tasks](kiali/) – service-mesh and observability workflows that exercise the Kiali MCP toolset (Istio config, topology, mesh health, tracing).
- [KubeVirt tasks](kubevirt/) – virtual machine management workflows that exercise the KubeVirt MCP toolset (VM creation, lifecycle management, resource updates).
- [Tekton tasks](tekton/) – CI/CD workflows that exercise the Tekton toolset (pipelines, tasks, pipeline runs).

## Anatomy of a Task

Most subdirectories under `core/`, `config/`, `helm/`, `kiali/`, `kubevirt/`, or `tekton/` define a single scenario (a few, like `kiali/scripts/`, `kubevirt/helpers/`, and `tekton/helpers/`, hold shared helpers instead):

1. `*.yaml` – declarative description consumed by the evaluation harness (prompts, success criteria, required tools).
2. `setup.sh` / `verify.sh` / `cleanup.sh` – shell hooks (optional) that prime the cluster, assert post-conditions, and reset resources so tasks stay idempotent.
3. `artifacts/` – supporting manifests, scripts, or payloads referenced by the task definition.

## Adding a New Task

1. Pick the closest family and create a new subfolder.
2. Author the task YAML referencing MCP tools, expected observations, and any artifacts.
3. Provide helper scripts if the scenario needs deterministic setup or verification.
4. Add project metadata so the task appears in the README validated-projects table (unlabeled tasks are skipped).
   - The toolset column comes from the `requires` label, not `project`; set `requires` when a scenario needs a non-core toolset.
   - `project` is a label; `project-name` and `project-url` are annotations. The generator panics if `project` is set without both.
   - Example: [helm/install-chart/install-chart.yaml](helm/install-chart/install-chart.yaml).
5. Document nuances in a local `README.md` so future contributors and eval authors can replay the scenario manually.

Well-scoped, deterministic tasks make it easier to compare agents and regressions over time, so keep inputs minimal, outputs explicit, and always clean up what you create.

## Adding a New Task Stack

When a new MCP toolset lands, keep its evaluations isolated by creating a sibling directory under `tasks/` named after the toolset (`tasks/<name>`, etc.). Populate it with:

1. A scoped `README.md` describing the toolset focus and prerequisite context.
2. One subfolder per scenario that follows the same layout described above (`*.yaml`, scripts, `artifacts/`).
3. Any shared fixtures the stack needs (place them in a `shared/` subdirectory if multiple scenarios reuse them).

This structure keeps task stacks discoverable and lets eval harnesses target toolset-specific workflows without mixing concerns from the core Kubernetes or Kiali libraries.
