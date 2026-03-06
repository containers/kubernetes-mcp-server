package kubernetes

import (
	"fmt"
	"slices"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

// ProviderFactory creates a new Provider instance for a given strategy.
// Implementations should validate that the Manager is compatible with their strategy
// (e.g., kubeconfig provider should reject in-cluster managers).
type ProviderFactory func(cfg api.BaseConfig) (Provider, error)

var providerReg = &providerRegistry{factories: make(map[string]ProviderFactory)}

// RegisterProvider registers a provider factory for a given strategy name.
// This should be called from init() functions in provider implementation files.
// Panics if a provider is already registered for the given strategy.
func RegisterProvider(strategy string, factory ProviderFactory) {
	providerReg.register(strategy, factory)
}

// getProviderFactory retrieves a registered provider factory by strategy name.
// Returns an error if no provider is registered for the given strategy.
func getProviderFactory(strategy string) (ProviderFactory, error) {
	return providerReg.get(strategy)
}

// GetRegisteredStrategies returns a sorted list of all registered strategy names.
// This is useful for error messages and debugging.
func GetRegisteredStrategies() []string {
	return providerReg.strategies()
}

type providerRegistry struct {
	factories map[string]ProviderFactory
}

func (r *providerRegistry) register(strategy string, factory ProviderFactory) {
	if _, exists := r.factories[strategy]; exists {
		panic(fmt.Sprintf("provider already registered for strategy '%s'", strategy))
	}
	r.factories[strategy] = factory
}

func (r *providerRegistry) get(strategy string) (ProviderFactory, error) {
	factory, ok := r.factories[strategy]
	if !ok {
		available := r.strategies()
		return nil, fmt.Errorf("no provider registered for strategy '%s', available strategies: %v", strategy, available)
	}
	return factory, nil
}

func (r *providerRegistry) strategies() []string {
	strategies := make([]string, 0, len(r.factories))
	for strategy := range r.factories {
		strategies = append(strategies, strategy)
	}
	slices.Sort(strategies)
	return strategies
}

func (r *providerRegistry) clear() {
	r.factories = make(map[string]ProviderFactory)
}
