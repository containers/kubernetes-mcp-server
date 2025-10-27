package config

import (
	"context"
	"fmt"

	"github.com/BurntSushi/toml"
)

// ProviderConfig is the interface that all provider-specific configurations must implement.
// Each provider registers a factory function to parse its config from TOML primitives
type ProviderConfig interface {
	Validate() error
}

type ProviderConfigParser func(ctx context.Context, primitive toml.Primitive, md toml.MetaData) (ProviderConfig, error)

type configDirPathKey struct{}

func withConfigDirPath(ctx context.Context, dirPath string) context.Context {
	return context.WithValue(ctx, configDirPathKey{}, dirPath)
}

func ConfigDirPathFromContext(ctx context.Context) string {
	val := ctx.Value(configDirPathKey{})

	if val == nil {
		return ""
	}

	if strVal, ok := val.(string); ok {
		return strVal
	}

	return ""
}

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
