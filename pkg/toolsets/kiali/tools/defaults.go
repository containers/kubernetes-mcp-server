package tools

// Default values for Kiali API parameters shared across this package.
const (
	// DefaultRateInterval is the default rate interval for fetching error rates and metrics.
	// This value is used when rateInterval is not explicitly provided in API calls.
	DefaultRateInterval    = "10m"
	DefaultGraphType       = "versionedApp"
	DefaultStep            = "15"
	DefaultDirection       = "outbound"
	DefaultReporter        = "source"
	DefaultQuantiles       = "0.5,0.95,0.99,0.999"
	DefaultLimit           = 10
	DefaultTail            = 50
	DefaultLookbackSeconds = 600
	DefaultErrorOnly       = false
)

// remapMeshCluster translates the MCP-facing "meshCluster" parameter to the
// Kiali API's "clusterName" before forwarding the request. This avoids a
// naming collision with the provider-level target parameter (e.g. "cluster"
// from an ACM provider) while keeping the Kiali backend API unchanged.
func remapMeshCluster(arguments map[string]any) map[string]any {
	if v, ok := arguments["meshCluster"]; ok {
		arguments["clusterName"] = v
		delete(arguments, "meshCluster")
	}
	return arguments
}
