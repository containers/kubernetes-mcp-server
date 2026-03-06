package kiali

// MeshHealthSummary represents aggregated health across the mesh
type MeshHealthSummary struct {
	OverallStatus    string                      `json:"overallStatus"` // HEALTHY, DEGRADED, UNHEALTHY
	Availability     float64                     `json:"availability"`  // Percentage 0-100
	TotalErrorRate   float64                     `json:"totalErrorRate"`
	NamespaceCount   int                         `json:"namespaceCount"`
	EntityCounts     EntityHealthCounts          `json:"entityCounts"`
	NamespaceSummary map[string]NamespaceSummary `json:"namespaceSummary"`
	TopUnhealthy     []UnhealthyEntity           `json:"topUnhealthy,omitempty"`
	Timestamp        string                      `json:"timestamp"`
	RateInterval     string                      `json:"rateInterval"`
}

// EntityHealthCounts contains health counts for all entity types
type EntityHealthCounts struct {
	Apps      HealthCounts `json:"apps"`
	Services  HealthCounts `json:"services"`
	Workloads HealthCounts `json:"workloads"`
}

// HealthCounts represents health status counts
type HealthCounts struct {
	Total     int `json:"total"`
	Healthy   int `json:"healthy"`
	Degraded  int `json:"degraded"`
	Unhealthy int `json:"unhealthy"`
	NotReady  int `json:"notReady"`
}

// NamespaceSummary contains health summary for a namespace
type NamespaceSummary struct {
	Status       string       `json:"status"`
	Availability float64      `json:"availability"`
	ErrorRate    float64      `json:"errorRate"`
	Apps         HealthCounts `json:"apps"`
	Services     HealthCounts `json:"services"`
	Workloads    HealthCounts `json:"workloads"`
}

// UnhealthyEntity represents an unhealthy entity
type UnhealthyEntity struct {
	Type      string  `json:"type"` // app, service, workload
	Namespace string  `json:"namespace"`
	Name      string  `json:"name"`
	Status    string  `json:"status"`
	Issue     string  `json:"issue"`
	ErrorRate float64 `json:"errorRate,omitempty"`
}

// ClustersNamespaceHealth matches Kiali's response structure
type ClustersNamespaceHealth struct {
	AppHealth      map[string]NamespaceAppHealth      `json:"namespaceAppHealth"`
	ServiceHealth  map[string]NamespaceServiceHealth  `json:"namespaceServiceHealth"`
	WorkloadHealth map[string]NamespaceWorkloadHealth `json:"namespaceWorkloadHealth"`
}

// NamespaceAppHealth is a map of app name to health
type NamespaceAppHealth map[string]AppHealth

// NamespaceServiceHealth is a map of service name to health
type NamespaceServiceHealth map[string]ServiceHealth

// NamespaceWorkloadHealth is a map of workload name to health
type NamespaceWorkloadHealth map[string]WorkloadHealth

// AppHealth contains health information for an app
type AppHealth struct {
	WorkloadStatuses []WorkloadStatus `json:"workloadStatuses"`
	Requests         RequestHealth    `json:"requests"`
}

// ServiceHealth contains health information for a service
type ServiceHealth struct {
	Requests RequestHealth `json:"requests"`
}

// WorkloadHealth contains health information for a workload
type WorkloadHealth struct {
	WorkloadStatus *WorkloadStatus `json:"workloadStatus"`
	Requests       RequestHealth   `json:"requests"`
}

// WorkloadStatus represents workload replica status
type WorkloadStatus struct {
	Name              string `json:"name"`
	DesiredReplicas   int32  `json:"desiredReplicas"`
	CurrentReplicas   int32  `json:"currentReplicas"`
	AvailableReplicas int32  `json:"availableReplicas"`
	SyncedProxies     int32  `json:"syncedProxies"`
}

// RequestHealth holds request health metrics
type RequestHealth struct {
	Inbound           map[string]map[string]float64 `json:"inbound"`
	Outbound          map[string]map[string]float64 `json:"outbound"`
	HealthAnnotations map[string]string             `json:"healthAnnotations"`
}
