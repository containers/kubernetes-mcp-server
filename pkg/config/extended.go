package config

import (
	"context"
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

type ExtendedConfigParser func(ctx context.Context, primitive toml.Primitive, md toml.MetaData) (api.ExtendedConfig, error)

type extendedConfigRegistry struct {
	parsers map[string]ExtendedConfigParser
}

func newExtendedConfigRegistry() *extendedConfigRegistry {
	return &extendedConfigRegistry{
		parsers: make(map[string]ExtendedConfigParser),
	}
}

func (r *extendedConfigRegistry) register(name string, parser ExtendedConfigParser) {
	if _, exists := r.parsers[name]; exists {
		panic("extended config parser already registered for name: " + name)
	}

	r.parsers[name] = parser
}

func (r *extendedConfigRegistry) parse(ctx context.Context, metaData toml.MetaData, configs map[string]toml.Primitive) (map[string]api.ExtendedConfig, error) {
	if len(configs) == 0 {
		return make(map[string]api.ExtendedConfig), nil
	}
	parsedConfigs := make(map[string]api.ExtendedConfig, len(configs))

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
