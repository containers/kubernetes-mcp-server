package kiali

// Default values for Kiali API parameters shared across this package.
const (
	// DefaultRateInterval is the default rate interval for fetching error rates and metrics.
	// This value is used when rateInterval is not explicitly provided in API calls.
	DefaultRateInterval = "10m"
)
