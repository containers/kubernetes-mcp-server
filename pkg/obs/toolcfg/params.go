// Package toolcfg resolves per-toolset ExtendedConfig for tool handlers.
//
// Standalone obs-mcp injects configs via context; embedders rely on
// params.Config populated from TOML.
package toolcfg

import (
	"context"
	"maps"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

type toolsetConfigKey struct{}

// With attaches a toolset ExtendedConfig to ctx.
func With(ctx context.Context, name string, cfg config.ExtendedConfig) context.Context {
	existing, _ := ctx.Value(toolsetConfigKey{}).(map[string]config.ExtendedConfig)
	m := maps.Clone(existing)
	if m == nil {
		m = make(map[string]config.ExtendedConfig, 1)
	}
	m[name] = cfg
	return context.WithValue(ctx, toolsetConfigKey{}, m)
}

// From returns toolset config from context, or params.Config if unset.
func From(params api.ToolHandlerParams, name string) (config.ExtendedConfig, bool) {
	if m, ok := params.Value(toolsetConfigKey{}).(map[string]config.ExtendedConfig); ok {
		if cfg, found := m[name]; found {
			return cfg, true
		}
	}
	if params.Config != nil {
		return params.Config.GetToolsetConfig(name)
	}
	return nil, false
}
