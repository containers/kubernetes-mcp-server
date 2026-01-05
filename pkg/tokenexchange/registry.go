package tokenexchange

var (
	exchangerRegistry = &tokenExchangerRegistry{exchangers: map[string]TokenExchanger{}}
)

func init() {
	RegisterTokenExchanger(StrategyKeycloakV1, &keycloakV1Exchanger{})
	RegisterTokenExchanger(StrategyRFC8693, &rfc8693Exchanger{})
}

func RegisterTokenExchanger(strategy string, exchanger TokenExchanger) {
	exchangerRegistry.register(strategy, exchanger)
}

func GetTokenExchanger(strategy string) (TokenExchanger, bool) {
	return exchangerRegistry.get(strategy)
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
