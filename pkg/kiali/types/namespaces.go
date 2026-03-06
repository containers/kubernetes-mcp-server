package kiali

// NamespaceListItemRaw is one namespace in the Kiali namespaces list API response.
type NamespaceListItemRaw struct {
	Name           string            `json:"name"`
	Cluster        string            `json:"cluster"`
	IsAmbient      bool              `json:"isAmbient"`
	IsControlPlane bool              `json:"isControlPlane"`
	Labels         map[string]string `json:"labels"`
	Annotations    map[string]string `json:"annotations"`
	Revision       string            `json:"revision"`
}

// NamespacesSummaryResponse is the formatted response for list namespaces with app health.
type NamespacesSummaryResponse struct {
	Namespaces []NamespaceSummaryFormatted `json:"namespaces"`
}

// NamespaceSummaryFormatted is one namespace in the summary output.
type NamespaceSummaryFormatted struct {
	Name    string                `json:"name"`
	Summary string                `json:"summary"`
	Apps    []AppSummaryFormatted `json:"apps"`
}

// AppSummaryFormatted is one app in a namespace summary.
type AppSummaryFormatted struct {
	Name          string `json:"name"`
	Status        string `json:"status"`
	SuccessRate   string `json:"success_rate,omitempty"`
	Replicas      string `json:"replicas,omitempty"`
	ProxiesSynced *bool  `json:"proxies_synced,omitempty"`
	Notes         string `json:"notes,omitempty"`
}

// --- Service health overview (type=service) ---

// ServiceHealthOverviewResponse is the formatted response when listing namespaces with service health.
type ServiceHealthOverviewResponse struct {
	ServiceHealthOverview []ServiceNamespaceOverview `json:"service_health_overview"`
}

// ServiceNamespaceOverview is one namespace in the service health overview.
type ServiceNamespaceOverview struct {
	Namespace        string                 `json:"namespace"`
	HealthyCount     int                    `json:"healthy_count,omitempty"`
	DegradedServices []ServiceDegradedEntry `json:"degraded_services,omitempty"`
	ActiveServices   []ServiceActiveEntry   `json:"active_services,omitempty"`
	Summary          string                 `json:"summary,omitempty"`
	Status           string                 `json:"status,omitempty"`
}

// ServiceDegradedEntry is a service with low success rate or issues.
type ServiceDegradedEntry struct {
	Name               string `json:"name"`
	InboundSuccessRate string `json:"inbound_success_rate"`
	Status             string `json:"status"`
	Issue              string `json:"issue"`
}

// ServiceActiveEntry is a healthy/active service.
type ServiceActiveEntry struct {
	Name        string `json:"name"`
	SuccessRate string `json:"success_rate"`
}

// --- Workload health overview (type=workload) ---

// WorkloadHealthOverviewResponse is the formatted response when listing namespaces with workload health.
type WorkloadHealthOverviewResponse struct {
	WorkloadHealth []WorkloadNamespaceOverview `json:"workload_health"`
}

// WorkloadNamespaceOverview is one namespace in the workload health overview.
type WorkloadNamespaceOverview struct {
	Namespace       string                  `json:"namespace"`
	CriticalIssues  []WorkloadCriticalIssue `json:"critical_issues,omitempty"`
	StableWorkloads []WorkloadStableEntry   `json:"stable_workloads,omitempty"`
	Status          string                  `json:"status,omitempty"`
	Workloads       []string                `json:"workloads,omitempty"`
}

// WorkloadCriticalIssue is a workload with degradation or low success rate.
type WorkloadCriticalIssue struct {
	Workload       string `json:"workload"`
	Issue          string `json:"issue"`
	InboundSuccess string `json:"inbound_success"`
	Status         string `json:"status"`
}

// WorkloadStableEntry is a healthy workload.
type WorkloadStableEntry struct {
	Name     string `json:"name"`
	Success  string `json:"success"`
	Replicas string `json:"replicas,omitempty"`
	Proxies  string `json:"proxies,omitempty"`
}
