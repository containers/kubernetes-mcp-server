package kubernetes

import (
	"fmt"
	"sort"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

// ProviderFactory creates a new Provider instance for a given strategy.
// Implementations should validate that the Manager is compatible with their strategy
// (e.g., kubeconfig provider should reject in-cluster managers).
type ProviderFactory func(cfg api.BaseConfig) (Provider, error)

var providerFactories = make(map[string]ProviderFactory)

// RegisterProvider registers a provider factory for a given strategy name.
// This should be called from init() functions in provider implementation files.
// Panics if a provider is already registered for the given strategy.
func RegisterProvider(strategy string, factory ProviderFactory) {
	if _, exists := providerFactories[strategy]; exists {
		panic(fmt.Sprintf("provider already registered for strategy '%s'", strategy))
	}
	providerFactories[strategy] = factory
}

// getProviderFactory retrieves a registered provider factory by strategy name.
// Returns an error if no provider is registered for the given strategy.
func getProviderFactory(strategy string) (ProviderFactory, error) {
	factory, ok := providerFactories[strategy]
	if !ok {
		available := GetRegisteredStrategies()
		return nil, fmt.Errorf("no provider registered for strategy '%s', available strategies: %v", strategy, available)
	}
	return factory, nil
}

// GetRegisteredStrategies returns a sorted list of all registered strategy names.
// This is useful for error messages and debugging.
func GetRegisteredStrategies() []string {
	strategies := make([]string, 0, len(providerFactories))
	for strategy := range providerFactories {
		strategies = append(strategies, strategy)
	}
	sort.Strings(strategies)
	return strategies
}
