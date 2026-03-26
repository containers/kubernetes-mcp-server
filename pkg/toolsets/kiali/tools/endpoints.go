package tools

// Kiali API endpoint paths shared across this package.
const (
	KialiMCPPath = "/api/chat/mcp"

	KialiGetMeshTrafficGraphEndpoint   = KialiMCPPath + "/get_mesh_traffic_graph"
	KialiGetMeshStatusEndpoint         = KialiMCPPath + "/get_mesh_status"
	KialiGetMetricsEndpoint            = KialiMCPPath + "/get_metrics"
	KialiListOrGetResourcesEndpoint    = KialiMCPPath + "/list_or_get_resources"
	KialiGetTracesEndpoint             = KialiMCPPath + "/get_traces"
	KialiGetLogsEndpoint               = KialiMCPPath + "/get_logs"
	KialiManageIstioConfigEndpoint     = KialiMCPPath + "/manage_istio_config"
	KialiManageIstioConfigReadEndpoint = KialiMCPPath + "/manage_istio_config_read"
	KialiGetPodPerformanceEndpoint     = KialiMCPPath + "/get_pod_performance"
)
