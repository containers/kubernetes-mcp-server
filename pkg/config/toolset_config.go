package config

import (
	"context"
	"fmt"

	"github.com/BurntSushi/toml"
)

// ToolsetConfig is the interface that all toolset-specific configurations must implement.
// Each toolset registers a factory function to parse its config from TOML primitives
type ToolsetConfig interface {
	Validate() error
}

type ToolsetConfigParser func(ctx context.Context, primitive toml.Primitive, md toml.MetaData) (ToolsetConfig, error)

var (
	toolsetConfigParsers = make(map[string]ToolsetConfigParser)
)

func RegisterToolsetConfig(name string, parser ToolsetConfigParser) {
	if _, exists := toolsetConfigParsers[name]; exists {
		panic(fmt.Sprintf("toolset config parser already registered for toolset '%s'", name))
	}

	toolsetConfigParsers[name] = parser
}

func getToolsetConfigParser(name string) (ToolsetConfigParser, bool) {
	parser, ok := toolsetConfigParsers[name]

	return parser, ok
}
