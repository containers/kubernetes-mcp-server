package tools

// filtersParameterDescription documents the NetObserv filters query language for MCP agents.
const filtersParameterDescription = `NetObserv filter expression passed to the console plugin (plain text; the client URL-encodes it).

Syntax:
- key=value — exact match; key=a,b — OR multiple values for the same key
- key~pattern — regex / contains match; key!~pattern — NOT regex
- key!=value — not equal; key>number — numeric greater-or-equal (e.g. Bytes>1000)
- AND within a group: & (e.g. SrcK8S_Namespace=default&Proto=6)
- OR between groups: | (e.g. SrcK8S_Name=pod-a|SrcK8S_Name=pod-b)

Prefer the dedicated "namespace" parameter for namespace scope when possible.
Use Kubernetes list tools (namespaces, pods, deployments, etc.) to discover filter values.

Common Kubernetes fields (Src/Dst prefixes mirror each other):
- SrcK8S_Namespace, DstK8S_Namespace, SrcK8S_Name, DstK8S_Name
- SrcK8S_Type, DstK8S_Type (e.g. Pod, Service, Node)
- SrcK8S_OwnerName, DstK8S_OwnerName, SrcK8S_OwnerType, DstK8S_OwnerType (for Deployment, StatefulSet, etc.)
- SrcK8S_HostName, DstK8S_HostName, SrcK8S_Zone, DstK8S_Zone, K8S_ClusterName, UDN

Network & flow:
- SrcAddr, DstAddr (IPs), SrcPort, DstPort, Proto (IANA number, e.g. 6=TCP, 17=UDP)
- FlowDirection (0=Ingress, 1=Egress, 2=Inner), Bytes, Packets, Dscp, Flags

Packet drops (often with recordType flowLog and packetLoss dropped/hasDrops):
- PktDropPackets, PktDropBytes, PktDropLatestState, PktDropLatestDropCause

DNS:
- DnsName, DnsId, DnsLatencyMs, DnsErrno, DnsFlagsResponseCode

Examples:
- SrcK8S_Namespace=openshift-netobserv&SrcK8S_Name~my-app
- Proto=6&DstPort=443
- SrcK8S_Name=pod-a|SrcK8S_Name=pod-b`
