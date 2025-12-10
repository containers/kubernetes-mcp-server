package config

import (
	"context"
	"fmt"

	"github.com/BurntSushi/toml"
	configapi "github.com/containers/kubernetes-mcp-server/pkg/api/config"
)

type ExtendedConfigParser func(ctx context.Context, primitive toml.Primitive, md toml.MetaData) (configapi.Extended, error)

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

func (r *extendedConfigRegistry) parse(ctx context.Context, metaData toml.MetaData, configs map[string]toml.Primitive) (map[string]configapi.Extended, error) {
	if len(configs) == 0 {
		return make(map[string]configapi.Extended), nil
	}
	parsedConfigs := make(map[string]configapi.Extended, len(configs))

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
