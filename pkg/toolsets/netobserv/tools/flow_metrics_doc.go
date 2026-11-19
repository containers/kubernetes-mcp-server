package tools

// aggregateByParameterDescription documents NetObserv flow metrics grouping for MCP agents.
const aggregateByParameterDescription = `Primary dimension for netobserv_get_flow_metrics (console plugin /api/flow/metrics).

Two forms (use exact spelling):

1) Topology scopes — aggregate endpoints for graph/topology views:
- app — application workloads (pods/services), excluding infrastructure traffic
- namespace — Kubernetes namespace (default)
- owner — controller owner (Deployment, StatefulSet, …)
- resource — pod, service, or node (finest workload granularity)
- host — node name
- zone — availability zone
- cluster — cluster name (multi-cluster)
- network — user-defined / secondary network name

2) Flow record fields — group by a single flow attribute (PascalCase field name).
Use for breakdown charts (TLS, DNS, drops, protocol). Field names match filters / flow logs.

TLS (requires TLS tracking on the FlowCollector):
- TLSVersion, TLSCipherSuite, TLSGroup, TLSTypes

DNS:
- DnsName, DnsFlagsResponseCode, DnsErrno

Packet drops:
- PktDropLatestState, PktDropLatestDropCause

Network / K8s (single-sided breakdown; pair with filters for src/dst):
- Proto, SrcPort, DstPort, FlowDirection, Dscp
- SrcK8S_Namespace, DstK8S_Namespace, SrcK8S_Name, DstK8S_Name
- SrcK8S_Type, DstK8S_Type, SrcK8S_OwnerName, DstK8S_OwnerName
- SrcK8S_HostName, DstK8S_HostName, SrcK8S_Zone, DstK8S_Zone
- K8S_ClusterName, SrcK8S_NetworkName, DstK8S_NetworkName

Pair aggregateBy with type and function:
- Throughput: type=Bytes or Packets, function=rate
- Flow count: type=Flows, function=count or rate
- DNS volume: type=DnsFlows, function=count
- DNS latency: type=DnsLatencyMs, function=avg, p90, or max
- RTT: type=TimeFlowRttNs, function=avg, min, or p90
- Drops: type=PktDropPackets or PktDropBytes, function=rate

Examples:
- aggregateBy=namespace, type=Bytes, function=rate
- aggregateBy=TLSVersion, type=Bytes, function=rate, filters=TLSTypes!~""
- aggregateBy=TLSGroup, type=Flows, function=count
- aggregateBy=DnsFlagsResponseCode, type=DnsFlows, function=count
- aggregateBy=PktDropLatestState, type=PktDropPackets, function=rate, packetLoss=dropped
- aggregateBy=resource, type=Bytes, function=rate, namespace=netobserv`

// groupsParameterDescription documents optional secondary grouping for flow metrics.
const groupsParameterDescription = `Optional comma-separated parent scopes when aggregateBy is a topology scope.
Adds extra label dimensions (e.g. break namespace results down by cluster or zone).
Ignored or less useful when aggregateBy is already a raw flow field (e.g. TLSVersion); use filters instead.

Single scopes:
- clusters, networks, zones, hosts, namespaces, owners

Combined scopes (use +, no spaces):
- clusters+zones, clusters+hosts, clusters+namespaces, clusters+owners
- zones+hosts, zones+namespaces, zones+owners
- hosts+namespaces, hosts+owners
- namespaces+owners
- networks+zones, networks+hosts, networks+namespaces, networks+owners

Examples:
- aggregateBy=namespace, groups=clusters
- aggregateBy=resource, groups=namespaces
- aggregateBy=owner, groups=zones,hosts`
