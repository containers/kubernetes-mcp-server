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

## observability/logs

| Tool | Converted | Notes |
|------|-----------|-------|
| `loki_list_instances` | yes | Route lookup failures are skipped per LokiStack instead of failing the tool; a Route without a host is treated as absent instead of an error. |
| `loki_label_names` | yes | Resolves the named LokiStack with a `get` instead of listing every LokiStack. `NOW±duration` accepts a single unit (`NOW-1h30m` is rejected). |
| `loki_label_values` | yes | As `loki_label_names`. |
| `loki_query_range` | yes | As `loki_label_names`. The `duration` argument accepts one unit, as its schema pattern always required. |

The `auth_mode = "kubeconfig"` option (query Loki with the server's own
token) has no equivalent: a Tool always sends the caller's token. The unused
`ServerPrompt` instructions constant had no consumer in-tree and was dropped.

## observability/metrics

| Tool | Converted | Notes |
|------|-----------|-------|
| `list_metrics` | yes | The regex is not compiled before the request; Prometheus reports an invalid one. |
| `execute_instant_query` | yes | No guardrails (see below). |
| `execute_range_query` | yes | No guardrails. Per-series statistics are computed in CEL (`sum`, `math.least`/`greatest`); `rangeQueryFullResponse` is a chart value. |
| `show_timeseries` | yes | The `olsUi` `_meta` travels through the new `spec.meta`. |
| `get_label_names` | yes |  |
| `get_label_values` | yes |  |
| `get_series` | yes |  |
| `get_alerts` | yes | Installed only when `alertmanagerUrl` is set (the in-tree tool failed at call time without `alertmanager_url`). The comma-separated `filter` becomes repeated `filter` parameters. |
| `get_silences` | yes | As `get_alerts`. |

Not converted: the PromQL **guardrails** (`guardrails`,
`max_metric_cardinality`, `max_label_cardinality`) and the metric-existence
check that ran before every query. Both parse PromQL with the Prometheus
parser and consult `/api/v1/status/tsdb`; a Tool cannot parse PromQL. The
tools are also cluster-aware now (they take the `context` argument in
multi-cluster mode), where the in-tree tools were not. The unused
`ServerPrompt` constant was dropped.

## observability/traces

| Tool | Converted | Notes |
|------|-----------|-------|
| `tempo_list_instances` | yes | Instances are listed even when their Route cannot be resolved (the in-tree tool skipped them). Service names are not DNS-sanitized (CR names already are). |
| `tempo_get_trace_by_id` | yes | A missing trace is reported from the response body as before; a 404 is reported as an HTTP error. |
| `tempo_search_traces` | yes | Limits are checked by `fail` steps with the in-tree messages. The 10 MiB response cap is the engine's 8 MiB. |
| `tempo_search_tags` | yes |  |
| `tempo_search_tag_values` | yes |  |

The `text/html` response check ("invalid authentication token") is not
replicated; such a response is returned as text. `auth_mode = "kubeconfig"`
has no equivalent. The unused `ServerPrompt` constant was dropped.

## observability/otelcol

| Tool | Converted | Notes |
|------|-----------|-------|
| `otelcol_list_components` | no | validates collector configuration against JSON schemas embedded in a Go module; no request to express |
| `otelcol_get_component_schema` | no | as above |
| `otelcol_validate_config` | no | as above |
| `otelcol_get_versions` | no | as above |

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

**73 of 92 tools and prompts converted.**
