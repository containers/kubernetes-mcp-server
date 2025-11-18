package config

import (
	"context"
	"fmt"

	"github.com/BurntSushi/toml"
)

// Extended is the interface that all configuration extensions must implement.
// Each extended config manager registers a factory function to parse its config from TOML primitives
type Extended interface {
	Validate() error
}

type ExtendedConfigParser func(ctx context.Context, primitive toml.Primitive, md toml.MetaData) (Extended, error)

type ExtendedConfigRegistry struct {
	parsers map[string]ExtendedConfigParser
}

func NewExtendedConfigRegistry() *ExtendedConfigRegistry {
	return &ExtendedConfigRegistry{
		parsers: make(map[string]ExtendedConfigParser),
	}
}

func (r *ExtendedConfigRegistry) Register(name string, parser ExtendedConfigParser) {
	if _, exists := r.parsers[name]; exists {
		panic("extended config parser already registered for name: " + name)
	}

	r.parsers[name] = parser
}

func (r *ExtendedConfigRegistry) Parse(ctx context.Context, metaData toml.MetaData, configs map[string]toml.Primitive) (map[string]Extended, error) {
	if len(configs) == 0 {
		return make(map[string]Extended), nil
	}
	parsedConfigs := make(map[string]Extended, len(configs))

	for name, primitive := range configs {
		parser, ok := r.parsers[name]
		if !ok {
			continue
		}

		extendedConfig, err := parser(ctx, primitive, metaData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse extended config for '%s': %w", name, err)
		}

		if err = extendedConfig.Validate(); err != nil {
			return nil, fmt.Errorf("failed to validate extended config for '%s': %w", name, err)
		}

		parsedConfigs[name] = extendedConfig
	}

	return parsedConfigs, nil
}
