# Kiali Task Stack

Kiali-focused MCP tasks live here. Each folder under this directory represents a self-contained scenario that exercises the Kiali toolset (Istio config, topology, observability, troubleshooting).

## Adding a New Task

1. Create a new subdirectory (e.g., `status-foo/`) and place the scenario YAML plus any helper scripts or artifacts inside it.
2. Make sure the YAML’s `metadata` block includes `name`, `category`, and `difficulty` so it shows up correctly in the catalog below.
3. Keep prompts concise and action-oriented; verification commands should rely on Kiali MCP tools whenever possible.

## Updating the Catalog

After adding or editing tasks, regenerate this README’s catalog with:

```bash
make update_tasks
```

The `update_tasks` target runs `scripts/update_tasks.sh`, which parses every scenario and rewrites the section below automatically. Always run it before committing so the list stays in sync.

## Tasks defined
<!-- TASKS-START -->
- High-Level Observability & Health
  - [easy] obs-unhealthy-namespaces (Unhealthy Namespaces)
        **Prompt:** *Are there any degraded namespaces in my mesh right now?*
  - [easy] show-topology (Show topology bookinfo)
        **Prompt:** *Show me the topology of the bookinfo namespace.*
  - [easy] status-kiali-istio (Status Kiali and Istio)
        **Prompt:** *Give me a status report on the interaction between Kiali and Istio components*
  - [easy] topology-mesh-namespaces (Show topology for multiple namespaces)
        **Prompt:** *Show me the topology for the bookinfo and default namespaces together.*
  - [easy] topology-workload-graph (Show workload-level topology)
        **Prompt:** *Show me the workload-level graph for the bookinfo namespace.*
- Istio Configuration & Management
  - [easy] istio-list (List all VS in bookinfo namespace)
        **Prompt:** *List all VirtualServices in the bookinfo namespace and check if they have any validation errors*
  - [easy] istio-list-destination-rules (List DestinationRules in namespace)
        **Prompt:** *List all DestinationRules in the bookinfo namespace and report any validation errors.*
  - [medium] istio-create (Create a gateway)
        **Prompt:** *Create a Gateway named my-gateway in the istio-system namespace.*
  - [medium] istio-delete (Remove fault Injection)
        **Prompt:** *Fix my namespace bookinfo to remove the fault injection.*
  - [medium] istio-patch (Patch my traffic)
        **Prompt:** *I need to shift 50% of traffic to v2 of the reviews service. Apply a patch to the VirtualService with name reviews. Not confirm the patch, just apply it.*
- Resource Inspection
  - [easy] metrics-service-request-rate (Metrics for service (request rate))
        **Prompt:** *Show me the request rate and error rate metrics for the productpage service in the bookinfo namespace over the last 10 minutes.*
  - [easy] metrics-workload-latency (Metrics for workload (latency))
        **Prompt:** *What are the latency metrics (p50, p95, p99) for the reviews workload in the bookinfo namespace? Use a 5-minute rate interval.*
  - [easy] resource-get-namespaces (Get mesh namespaces)
        **Prompt:** *Check namespaces in my mesh.*
  - [easy] resource-get-service-detail (Get service detail)
        **Prompt:** *Get the full details for the service reviews in the namespace bookinfo.*
  - [easy] resource-get-workload-detail (Get workload detail)
        **Prompt:** *Get the full details and health status for the reviews-v1 workload in the bookinfo namespace.*
  - [easy] resource-list-services (List services in namespace)
        **Prompt:** *List all services in the bookinfo namespace.*
  - [easy] resource-list-workloads (List workloads without sidecar)
        **Prompt:** *List all workloads in the bookinfo namespace that have missing sidecars.*
  - [easy] resource-mesh-status (Status of my mesh)
        **Prompt:** *Check my mesh.*
- Troubleshooting & Debugging
  - [easy] troubleshooting-latency-traces (Get latency workload)
        **Prompt:** *Analyze the latency for the reviews workload over the last 30 minutes?*
  - [easy] troubleshooting-log (Get log productpage due 500)
        **Prompt:** *Why is the productpage service returning 500 errors?*
  - [easy] troubleshooting-trace-lagging (Check traces for a service)
        **Prompt:** *I see a spike in duration for ratings. Can you check the traces to see which span is lagging?*
  - [easy] troubleshooting-traces-by-app (Get traces for an app)
        **Prompt:** *Get the latest traces for the productpage app in the bookinfo namespace.*
  - [easy] troubleshooting-workload-logs (Get workload logs)
        **Prompt:** *Show me the last 20 log lines for the productpage-v1 workload in the bookinfo namespace.*
<!-- TASKS-END -->


<!-- SUMMARY-OUTPUT-START -->
=== Evaluation Summary ===

  ✓ get-namespaces (assertions: 3/3)
  ✓ get-service-detail (assertions: 3/3)
  ✓ Create a gateway (assertions: 3/3)
  ✓ Remove fault Injection (assertions: 3/3)
  ✓ List all VS in bookinfo namespace (assertions: 3/3)
  ✓ List DestinationRules in namespace (assertions: 3/3)
  ✓ Patch my traffic (assertions: 3/3)
  ✓ mesh-status (assertions: 3/3)
  ✓ Metrics for service (request rate) (assertions: 3/3)
  ✓ Metrics for workload (latency) (assertions: 3/3)
  ✓ Unhealthy Namespaces (assertions: 3/3)
  ✓ Get mesh namespaces (assertions: 3/3)
  ✓ Get service detail (assertions: 3/3)
  ✓ Get workload detail (assertions: 3/3)
  ✓ List services in namespace (assertions: 3/3)
  ✓ List workloads without sidecar (assertions: 3/3)
  ✓ Status of my mesh (assertions: 3/3)
  ✓ Show topology bookinfo (assertions: 3/3)
  ✓ Status Kiali and Istio (assertions: 3/3)
  ✓ Show topology for multiple namespaces (assertions: 3/3)
  ✓ Show workload-level topology (assertions: 3/3)
  ✓ Get latency workload (assertions: 3/3)
  ✓ Get log productpage due 500 (assertions: 3/3)
  ✓ Check traces for a service (assertions: 3/3)
  ✓ Get traces for an app (assertions: 3/3)
  ✓ Get workload logs (assertions: 3/3)

Tasks:      26/26 passed (100.00%)
Assertions: 78/78 passed (100.00%)
Tokens:     ~126394 (incomplete - some counts failed)
MCP schemas: ~43628 (included in token total)
Judge used tokens:
  Input:  22348 tokens
  Output: 1479 tokens
<!-- SUMMARY-OUTPUT-END -->
