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
- Configuration Management
  - [easy] istio-list (List and Validate VirtualServices)
        **Prompt:** *|*
  - [easy] istio-list-destination-rules (List and Validate DestinationRules)
        **Prompt:** *"List all DestinationRules in the 'bookinfo' namespace and report if any of them have validation errors."*
  - [medium] istio-create (Create Istio Gateway)
        **Prompt:** *"Create a new Istio Gateway named 'my-gateway' in the 'istio-system' namespace."*
  - [medium] istio-delete (Remove Fault Injection)
        **Prompt:** *"The 'bookinfo' namespace seems to have some fault injection configured that's causing issues. Please identify and remove any fault injection from the 'ratings' VirtualService."*
  - [medium] istio-patch (Update Traffic Shifting)
        **Prompt:** *"I need to implement a 50/50 traffic split between versions for the 'reviews' service in the 'bookinfo' namespace. Apply the necessary patch to the 'reviews' VirtualService without asking for confirmation."*
- Mesh Health & Status
  - [easy] obs-unhealthy-namespaces (Identify Degraded Namespaces)
        **Prompt:** *"Check the health of my mesh and let me know if there are any unhealthy namespaces right now."*
  - [easy] resource-mesh-status (Comprehensive Mesh Health Audit)
        **Prompt:** *|*
  - [easy] status-kiali-istio (Audit Control Plane Connectivity)
        **Prompt:** *"Report on the current status and connectivity between Kiali and the Istio control plane components."*
- Performance Analysis
  - [easy] metrics-service-request-rate (Analyze Service Traffic Volume)
        **Prompt:** *|*
  - [easy] metrics-workload-latency (Analyze Workload Response Times)
        **Prompt:** *|*
- Resource Inspection
  - [easy] resource-get-namespaces (List Mesh-Enabled Namespaces)
        **Prompt:** *"Provide a list of all namespaces currently included in my Istio service mesh."*
  - [easy] resource-get-service-detail (Inspect Service Details)
        **Prompt:** *"Get the full configuration details and current health status for the 'reviews' service in the 'bookinfo' namespace."*
  - [easy] resource-get-workload-detail (Inspect Workload Details)
        **Prompt:** *"Inspect the 'reviews-v1' workload in the 'bookinfo' namespace and provide its detailed status and health information."*
  - [easy] resource-list-services (Inventory Namespace Services)
        **Prompt:** *"List all services available in the 'bookinfo' namespace."*
  - [easy] resource-list-workloads (Inventory Workloads with Sidecar Status)
        **Prompt:** *"Identify any workloads in the 'bookinfo' namespace that are missing the Istio sidecar proxy."*
- Traffic Observability
  - [easy] show-topology (Visualize Namespace Traffic)
        **Prompt:** *"Show me the traffic topology graph for the 'bookinfo' namespace."*
  - [easy] topology-mesh-namespaces (Visualize Cross-Namespace Traffic)
        **Prompt:** *|*
  - [easy] topology-workload-graph (Visualize Workload-Level Topology)
        **Prompt:** *|*
- Troubleshooting & Diagnostics
  - [easy] troubleshooting-log (Debug Service Errors via Logs)
        **Prompt:** *|*
  - [easy] troubleshooting-trace-lagging (Analyze Latency with Distributed Tracing)
        **Prompt:** *|*
  - [easy] troubleshooting-workload-logs (Retrieve Recent Workload Logs)
        **Prompt:** *"Retrieve the last 20 log lines for the 'productpage-v1' workload in the 'bookinfo' namespace."*
<!-- TASKS-END -->


<!-- SUMMARY-OUTPUT-START -->
=== Evaluation Summary ===

  ✓ Create Istio Gateway (assertions: 3/3)
  ✓ Remove Fault Injection (assertions: 3/3)
  ✓ List and Validate VirtualServices (assertions: 3/3)
  ✓ List and Validate DestinationRules (assertions: 3/3)
  ✓ Update Traffic Shifting (assertions: 3/3)
  ✓ Analyze Service Traffic Volume (assertions: 3/3)
  ✓ Analyze Workload Response Times (assertions: 3/3)
  ✓ Identify Degraded Namespaces (assertions: 3/3)
  ✓ List Mesh-Enabled Namespaces (assertions: 3/3)
  ✓ Inspect Service Details (assertions: 3/3)
  ✓ Inspect Workload Details (assertions: 3/3)
  ✓ Inventory Namespace Services (assertions: 3/3)
  ✓ Inventory Workloads with Sidecar Status (assertions: 3/3)
  ✓ Comprehensive Mesh Health Audit (assertions: 3/3)
  ✓ Visualize Namespace Traffic (assertions: 3/3)
  ✓ Audit Control Plane Connectivity (assertions: 3/3)
  ✓ Visualize Cross-Namespace Traffic (assertions: 3/3)
  ✓ Visualize Workload-Level Topology (assertions: 3/3)
  ✓ Debug Service Errors via Logs (assertions: 3/3)
  ✓ Analyze Latency with Distributed Tracing (assertions: 3/3)
  ✓ Retrieve Recent Workload Logs (assertions: 3/3)

Tasks: 21/21 passed (100.00%)
Assertions: 63/63 passed (100.00%)
Tokens: ~82147 (incomplete - some counts failed)
MCP schemas: ~59787 (included in token total)
Judge used tokens:
Input: 98980 tokens
Output: 3199 tokens
<!-- SUMMARY-OUTPUT-END -->
