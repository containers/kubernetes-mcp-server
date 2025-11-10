package kiali

// Kiali API endpoint paths shared across this package.
const (
	// MeshGraph is the Kiali API path that returns the mesh graph/status.
	AuthInfoEndpoint          = "/api/auth/info"
	MeshGraphEndpoint         = "/api/mesh/graph"
	GraphEndpoint             = "/api/namespaces/graph"
	HealthEndpoint            = "/api/clusters/health"
	IstioConfigEndpoint       = "/api/istio/config"
	IstioObjectEndpoint       = "/api/namespaces/%s/istio/%s/%s/%s/%s"
	IstioObjectCreateEndpoint = "/api/namespaces/%s/istio/%s/%s/%s"
	NamespacesEndpoint        = "/api/namespaces"
	PodDetailsEndpoint        = "/api/namespaces/%s/pods/%s"
	PodsLogsEndpoint          = "/api/namespaces/%s/pods/%s/logs"
	ServicesEndpoint          = "/api/clusters/services"
	ServiceDetailsEndpoint    = "/api/namespaces/%s/services/%s"
	ServiceMetricsEndpoint    = "/api/namespaces/%s/services/%s/metrics"
	AppTracesEndpoint         = "/api/namespaces/%s/apps/%s/traces"
	ServiceTracesEndpoint     = "/api/namespaces/%s/services/%s/traces"
	WorkloadTracesEndpoint    = "/api/namespaces/%s/workloads/%s/traces"
	WorkloadsEndpoint         = "/api/clusters/workloads"
	WorkloadDetailsEndpoint   = "/api/namespaces/%s/workloads/%s"
	WorkloadMetricsEndpoint   = "/api/namespaces/%s/workloads/%s/metrics"
	ValidationsEndpoint       = "/api/istio/validations"
	ValidationsListEndpoint   = "/api/istio/validations"
)
