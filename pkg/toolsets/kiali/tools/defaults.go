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
	DefaultLimit           = "10"
	DefaultTail            = "50"
	DefaultLookbackSeconds = 600
	DefaultErrorOnly       = "false"
)
