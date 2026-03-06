package kiali

// IstioListSummaryFormatted is the simplified Istio config list response.
type IstioListSummaryFormatted struct {
	Summary          IstioListSummary           `json:"summary"`
	ActiveResources  []IstioListActiveResource  `json:"active_resources"`
	ValidationAlerts []IstioListValidationAlert `json:"validation_alerts"`
}

// IstioListSummary holds total object count and unhealthy namespaces.
type IstioListSummary struct {
	TotalObjects        int      `json:"total_objects"`
	UnhealthyNamespaces []string `json:"unhealthy_namespaces"`
}

// IstioListActiveResource is one resource kind with non-zero count.
type IstioListActiveResource struct {
	Kind   string   `json:"kind"`
	Group  string   `json:"group"`
	Count  int      `json:"count"`
	Status string   `json:"status"` // "ok", "warning", "error"
	Items  []string `json:"items"`
}

// IstioListValidationAlert is per-namespace validation summary (only when warnings or errors > 0).
type IstioListValidationAlert struct {
	Namespace string `json:"namespace"`
	Cluster   string `json:"cluster"`
	Warnings  int    `json:"warnings"`
	Errors    int    `json:"errors"`
}

// --- Istio config GET (single object) simplified response ---

// IstioConfigGetFormatted is the simplified get-Istio-object response.
type IstioConfigGetFormatted struct {
	Summary     IstioConfigGetSummary     `json:"summary"`
	Diagnostics IstioConfigGetDiagnostics `json:"diagnostics"`
	Relations   IstioConfigGetRelations   `json:"relations"`
	YAMLRaw     string                    `json:"yaml_raw"`
}

// IstioConfigGetSummary holds name, namespace, kind, status, permissions.
type IstioConfigGetSummary struct {
	Name        string   `json:"name"`
	Namespace   string   `json:"namespace"`
	Kind        string   `json:"kind"`
	Status      string   `json:"status"` // "ok", "warning", "error"
	Permissions []string `json:"permissions"`
}

// IstioConfigGetDiagnostics holds validation result and issues.
type IstioConfigGetDiagnostics struct {
	Valid  bool                  `json:"valid"`
	Issues []IstioConfigGetIssue `json:"issues"`
}

// IstioConfigGetIssue is one validation check.
type IstioConfigGetIssue struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Location string `json:"location"`
}

// IstioConfigGetRelations holds gateways and services referenced by the object.
type IstioConfigGetRelations struct {
	Gateways []string `json:"gateways"`
	Services []string `json:"services"`
}
