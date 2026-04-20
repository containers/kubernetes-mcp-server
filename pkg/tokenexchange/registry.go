package tokenexchange

import "slices"

var (
	exchangerRegistry = &tokenExchangerRegistry{exchangers: map[string]TokenExchanger{}}
)

func init() {
	RegisterTokenExchanger(StrategyKeycloakV1, &keycloakV1Exchanger{})
	RegisterTokenExchanger(StrategyRFC8693, &rfc8693Exchanger{})
	RegisterTokenExchanger(StrategyEntraOBO, &entraOBOExchanger{})
}

func RegisterTokenExchanger(strategy string, exchanger TokenExchanger) {
	exchangerRegistry.register(strategy, exchanger)
}

func GetTokenExchanger(strategy string) (TokenExchanger, bool) {
	return exchangerRegistry.get(strategy)
}

// GetRegisteredStrategies returns a sorted list of all registered token exchange
// strategy names. Useful for config validation and error messages.
func GetRegisteredStrategies() []string {
	return exchangerRegistry.strategies()
}

type tokenExchangerRegistry struct {
	exchangers map[string]TokenExchanger
}

func (r *tokenExchangerRegistry) register(strategy string, exchanger TokenExchanger) {
	if _, exists := r.exchangers[strategy]; exists {
		panic("token exchanger already registered for strategy " + strategy)
	}

	r.exchangers[strategy] = exchanger
}

func (r *tokenExchangerRegistry) get(strategy string) (TokenExchanger, bool) {
	exchanger, ok := r.exchangers[strategy]
	return exchanger, ok
}

func (r *tokenExchangerRegistry) strategies() []string {
	strategies := make([]string, 0, len(r.exchangers))
	for strategy := range r.exchangers {
		strategies = append(strategies, strategy)
	}
	slices.Sort(strategies)
	return strategies
}
