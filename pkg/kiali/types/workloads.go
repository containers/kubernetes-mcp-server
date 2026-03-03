package kiali

// WorkloadsListResponse is the raw response from Kiali workloads list API.
// Used only for unmarshaling; transform to WorkloadsByCluster for output.
type WorkloadsListResponse struct {
	Cluster     string               `json:"cluster"`
	Workloads   []WorkloadListItem   `json:"workloads"`
	Validations WorkloadsValidations `json:"validations"`
}

// WorkloadListItem is one workload entry in the workloads list API response.
type WorkloadListItem struct {
	Name         string             `json:"name"`
	Namespace    string             `json:"namespace"`
	Cluster      string             `json:"cluster"`
	GVK          WorkloadGVK        `json:"gvk"`
	Labels       map[string]string  `json:"labels"`
	Health       WorkloadListHealth `json:"health"`
	IstioRefs    []IstioRef         `json:"istioReferences"`
	AppLabel     bool               `json:"appLabel"`
	VersionLabel bool               `json:"versionLabel"`
	IstioSidecar bool               `json:"istioSidecar"`
}

// WorkloadGVK holds Group, Version, Kind for the workload.
type WorkloadGVK struct {
	Group   string `json:"Group"`
	Version string `json:"Version"`
	Kind    string `json:"Kind"`
}

// WorkloadListHealth holds health status from the workloads list API (health.status.status).
type WorkloadListHealth struct {
	Status WorkloadHealthStatus `json:"status"`
}

// WorkloadHealthStatus holds the status string (e.g. "NA", "Healthy", "Degraded").
type WorkloadHealthStatus struct {
	Status string `json:"status"`
}

// WorkloadsValidations holds per-workload validation; key is "name.namespace".
type WorkloadsValidations struct {
	Workload map[string]WorkloadValidation `json:"workload"`
}

// WorkloadValidation holds whether the workload config is valid and any validation checks.
type WorkloadValidation struct {
	Valid  bool    `json:"valid"`
	Checks []Check `json:"checks"`
}

// WorkloadSummary is the simplified workload info returned to callers (e.g. MCP tools).
type WorkloadSummary struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Health    string `json:"health"`
	Type      string `json:"type"`   // gvk.Kind (e.g. Deployment)
	Labels    string `json:"labels"` // key=value pairs or "None"
	Details   string `json:"details"`
}

// WorkloadsByCluster groups workload summaries by cluster name.
type WorkloadsByCluster map[string][]WorkloadSummary
