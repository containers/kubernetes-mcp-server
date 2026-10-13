# Conversion report

Final report of the migration from in-tree Go toolsets to Tool and Prompt resources (the exporter that produced it was removed with the toolsets). One row per former in-tree tool or prompt.

## kiali

| Tool | Converted | Notes |
|------|-----------|-------|
| `kiali_get_mesh_traffic_graph` | yes |  |
| `kiali_get_mesh_status` | yes |  |
| `kiali_manage_istio_config_read` | yes |  |
| `kiali_manage_istio_config` | yes |  |
| `kiali_list_mesh_clusters` | yes |  |
| `kiali_get_resource_details` | yes |  |
| `kiali_list_traces` | yes |  |
| `kiali_get_trace_details` | yes |  |
| `kiali_get_pod_performance` | yes |  |
| `kiali_get_logs` | yes |  |
| `kiali_get_metrics` | yes |  |
| `mesh-list-applications (prompt)` | yes |  |
| `list-istio-config (prompt)` | yes |  |
| `mesh-list-namespaces (prompt)` | yes |  |
| `mesh-list-services (prompt)` | yes |  |
| `mesh-list-workloads (prompt)` | yes |  |
| `mesh-health-check (prompt)` | yes |  |
| `mesh-topology (prompt)` | yes |  |
| `traffic-topology (prompt)` | yes |  |
| `service-troubleshoot (prompt)` | yes |  |
| `trace-analysis (prompt)` | yes |  |
| `istio-config-review (prompt)` | yes |  |

## netobserv

| Tool | Converted | Notes |
|------|-----------|-------|
| `netobserv_list_flows` | yes | Response size is capped at 8 MiB instead of 4 MiB. |
| `netobserv_get_flow_metrics` | yes | Response size is capped at 8 MiB instead of 4 MiB. |
| `netobserv_export_flows` | yes | A response above 8 MiB is cut instead of being annotated as truncated. |

## tekton

| Tool | Converted | Notes |
|------|-----------|-------|
| `tekton_pipeline_start` | yes | Does not check that the Pipeline exists before creating the run. |
| `tekton_pipelinerun_lifecycle` | yes | Restart uses the same get + create as the built-in tool, expressed as two steps. |
| `tekton_pipelinerun_logs` | yes | Each container's logs are one fan-out request instead of a stream; empty logs are not skipped and the per-TaskRun grouping is flattened into one header per container. |
| `tekton_task_start` | yes | Does not check that the Task exists before creating the run. |
| `tekton_taskrun_restart` | yes |  |
| `tekton_taskrun_logs` | yes | Same differences as tekton_pipelinerun_logs. |
| `pipeline-troubleshoot (prompt)` | yes | Failed-step logs are redacted; the TaskRun section is a projection of the built-in text table. |

## kubevirt

| Tool | Converted | Notes |
|------|-----------|-------|
| `vm_clone` | yes |  |
| `vm_console_screenshot` | yes | The PNG is returned as an image content block; the description omits the pixel dimensions the built-in tool decodes. |
| `vm_create` | no | replaced by the `vm-create` prompt, which hands the model the DataSources, instancetypes, preferences and templates the tool matched heuristically, plus `vm_create_from_template` |
| `vm_guest_info` | yes | Same contract as the built-in tool; `all` fans out over the four subresources and tolerates individual failures. |
| `vm_lifecycle` | yes | Same contract as the built-in tool. runStrategy changes are merge patches instead of full-object updates. |
| `vm_create_from_template` | yes |  |
| `vm_troubleshoot` | yes | Gathers VM, VMI, virt-launcher pods, their logs (secrets redacted), events and DataVolumes. Omits the PVC fallback and the cloud-init analysis; cloud-init volumes are redacted wholesale. |
| `vm-troubleshoot (prompt)` | yes | Pod logs are included with secrets redacted; cloud-init volumes are redacted wholesale instead of field by field. |
| `windows-golden-image (prompt)` | yes | The Tekton Hub catalog name is fixed to the upstream default; downstream builds override it in Go. |
| `hco-status (prompt)` | yes |  |
| `vm-create (prompt)` | new | New: hands the model the candidates the vm_create tool matched heuristically. |

## kcp

| Tool | Converted | Notes |
|------|-----------|-------|
| `kcp_workspaces_list` | no | redundant: the kcp cluster provider already exposes every workspace as a target through `workspace_list` |
| `kcp_workspace_describe` | yes |  |

## core

| Tool | Converted | Notes |
|------|-----------|-------|
| `events_list` | yes |  |
| `namespaces_list` | yes | The table output format (list_output) is not available to dynamic tools. |
| `projects_list` | yes |  |
| `nodes_log` | yes |  |
| `nodes_stats_summary` | yes |  |
| `nodes_top` | yes | A name resolves to a GET by path; the RBAC metadata still advertises list. |
| `pods_list` | yes | The table output format (list_output) is not available to dynamic tools. |
| `pods_list_in_namespace` | yes | The table output format (list_output) is not available to dynamic tools. |
| `pods_get` | yes |  |
| `pods_delete` | yes | Does not clean up the Service that pods_run may have created alongside the Pod. |
| `pods_top` | yes | A name resolves to a GET by path; the RBAC metadata still advertises list. Omitting namespace lists all namespaces regardless of all_namespaces. |
| `pods_exec` | no | SPDY/WebSocket stream |
| `pods_log` | yes | Same contract as the built-in tool: without a container argument the Pod is fetched and its default-container annotation or first container used. |
| `pods_run` | yes | Does not create the OpenShift Route the built-in tool adds when the Route API is available; the generated name uses a timestamp instead of a random suffix. |
| `resources_list` | no | generic by design: the GVK is an argument; this is the base every dynamic tool builds on |
| `resources_get` | no | generic by design |
| `resources_create_or_update` | no | generic by design, multi-document manifests |
| `resources_delete` | no | generic by design |
| `resources_scale` | no | generic by design (see examples/dynamic-tools/tools/deployment-scale.yaml for a fixed-kind version) |
| `cluster-health-check (prompt)` | no | base server prompt, stays in-tree |

## config

| Tool | Converted | Notes |
|------|-----------|-------|
| `configuration_contexts_list` | no | reads the server's kubeconfig; target listing belongs to the provider |
| `targets_list` | no | provided by the cluster provider |
| `configuration_view` | no | reads the server's kubeconfig, not the cluster |

## helm

| Tool | Converted | Notes |
|------|-----------|-------|
| `helm_install` | no | Helm release storage and chart rendering are not Kubernetes API requests |
| `helm_list` | no | reads Helm release storage |
| `helm_uninstall` | no | Helm release storage |

**55 of 70 tools and prompts converted.**
