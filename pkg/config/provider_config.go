package config

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

// ProviderConfig is the interface that all provider-specific configurations must implement.
// Each provider registers a factory function to parse its config from TOML primitives
type ProviderConfig interface {
	Validate() error
}

type ProviderConfigParser func(primitive toml.Primitive, md toml.MetaData) (ProviderConfig, error)

var (
	providerConfigParsers = make(map[string]ProviderConfigParser)
)

func RegisterProviderConfig(strategy string, parser ProviderConfigParser) {
	if _, exists := providerConfigParsers[strategy]; exists {
		panic(fmt.Sprintf("provider config parser already registered for strategy '%s'", strategy))
	}

	providerConfigParsers[strategy] = parser
}

func getProviderConfigParser(strategy string) (ProviderConfigParser, bool) {
	provider, ok := providerConfigParsers[strategy]

	return provider, ok
}
