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
        **Prompt:** *Are there any unhealthy namespaces in my mesh right now?*
  - [easy] show-topology (Show topology bookinfo)
        **Prompt:** *Show me the topology of the bookinfo namespace.*
  - [easy] status-kiali-istio (Status Kiali and Istio)
        **Prompt:** *Give me a status report on the interaction between Kiali and Istio components*
- Istio Configuration & Management
  - [easy] istio-list (List all VS in bookinfo namespace)
        **Prompt:** *List all VirtualServices in the bookinfo namespace and check if they have any validation errors*
  - [medium] istio-create (Create a gateway)
        **Prompt:** *Create a Gateway named my-gateway in the istio-system namespace.*
  - [medium] istio-delete (Remove fault Injection)
        **Prompt:** *Fix my namespace bookinfo to remove the fault injection.*
  - [medium] istio-patch (Patch my traffic)
        **Prompt:** *I need to shift 50% of traffic to v2 of the reviews service. Apply a patch to the existing VirtualService.*
- Resource Inspection
  - [easy] resource-get-namespaces (Get mesh namespaces)
        **Prompt:** *Check namespaces in my mesh.*
  - [easy] resource-get-service-detail (Get service detail)
        **Prompt:** *Get the full details and health status for the details service*
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
- Uncategorized
  - [easy] delete-faultInjection (Remove fault Injection)
        **Prompt:** *Fix my namespace bookinfo to remove the fault injection.*
  - [easy] get-namespaces (get-namespaces)
        **Prompt:** *Check namespaces in my mesh.*
  - [easy] get-service-detail (get-service-detail)
        **Prompt:** *Give me information about my service details in the namespace bookinfo.*
  - [easy] mesh-status (mesh-status)
        **Prompt:** *Check my mesh.*
<!-- TASKS-END -->
