package kiali

// MeshSummaryFormatted is the simplified mesh status output for GetMeshStatus.
type MeshSummaryFormatted struct {
	Environment       MeshSummaryEnvironment     `json:"environment"`
	Components        MeshSummaryComponents      `json:"components"`
	ConnectivityGraph []MeshSummaryEdge          `json:"connectivity_graph"`
	CriticalAlerts    []MeshSummaryCriticalAlert `json:"critical_alerts,omitempty"`
}

// MeshSummaryEnvironment holds trust domain, versions, and timestamp.
type MeshSummaryEnvironment struct {
	TrustDomain  string `json:"trust_domain"`
	IstioVersion string `json:"istio_version"`
	KialiVersion string `json:"kiali_version"`
	Timestamp    string `json:"timestamp"`
}

// MeshSummaryComponents groups control plane, observability, and data plane.
type MeshSummaryComponents struct {
	ControlPlane       MeshSummaryControlPlane       `json:"control_plane"`
	ObservabilityStack MeshSummaryObservabilityStack `json:"observability_stack"`
	DataPlane          MeshSummaryDataPlane          `json:"data_plane"`
}

// MeshSummaryControlPlane holds istiod status and node names.
type MeshSummaryControlPlane struct {
	Status string   `json:"status"`
	Nodes  []string `json:"nodes"`
}

// MeshSummaryObservabilityStack holds status of Prometheus, Jaeger, Grafana.
type MeshSummaryObservabilityStack struct {
	Prometheus string `json:"prometheus"`
	Jaeger     string `json:"jaeger,omitempty"`
	Grafana    string `json:"grafana,omitempty"`
	Tempo      string `json:"tempo,omitempty"`
	OTel       string `json:"otel,omitempty"`
	Zipkin     string `json:"zipkin,omitempty"`
}

// MeshSummaryDataPlane holds monitored namespaces and injection info.
type MeshSummaryDataPlane struct {
	MonitoredNamespaces []string `json:"monitored_namespaces"`
	IstioInjection      string   `json:"istio_injection"`
}

// MeshSummaryEdge is one edge in the connectivity graph.
type MeshSummaryEdge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Status string `json:"status"`
	Note   string `json:"note,omitempty"`
}

// MeshSummaryCriticalAlert is a critical issue to surface.
type MeshSummaryCriticalAlert struct {
	Impact  string `json:"impact"`
	Message string `json:"message"`
}
