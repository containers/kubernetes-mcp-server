package config

import (
	"bytes"

	"github.com/BurntSushi/toml"
)

func Default() *StaticConfig {
	defaultConfig := StaticConfig{
		ListOutput: "table",
		Toolsets:   []string{"core", "config", "helm"},
	}
	overrides := defaultOverrides()
	mergedConfig := mergeConfig(defaultConfig, overrides)
	return &mergedConfig
}

// HasDefaultOverrides indicates whether the internal defaultOverrides function
// provides any overrides or an empty StaticConfig.
func HasDefaultOverrides() bool {
	overrides := defaultOverrides()
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(overrides); err != nil {
		// If marshaling fails, assume no overrides
		return false
	}
	return len(bytes.TrimSpace(buf.Bytes())) > 0
}

// mergeConfig applies non-zero values from override to base using TOML serialization
// and returns the merged StaticConfig.
// In case of any error during marshalling or unmarshalling, it returns the base config unchanged.
func mergeConfig(base, override StaticConfig) StaticConfig {
	var overrideBuffer bytes.Buffer
	if err := toml.NewEncoder(&overrideBuffer).Encode(override); err != nil {
		// If marshaling fails, return base unchanged
		return base
	}

	_, _ = toml.NewDecoder(&overrideBuffer).Decode(&base)
	return base
}
