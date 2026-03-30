package config

import (
	"bytes"
	"time"

	"github.com/BurntSushi/toml"
)

// BaseDefault returns the upstream base defaults before any
// build-time overrides are applied. This is useful for understanding
// the raw upstream configuration independent of downstream customization.
func BaseDefault() *StaticConfig {
	return &StaticConfig{
		ListOutput:           "table",
		Toolsets:             []string{"core", "config"},
		ConfirmationFallback: "allow",
		HTTP: HTTPConfig{
			ReadTimeout:       Duration(30 * time.Second),
			IdleTimeout:       Duration(60 * time.Second), // Per Apache recommendation
			ReadHeaderTimeout: Duration(10 * time.Second), // Slowloris protection
			MaxHeaderBytes:    1 << 20,                    // 1 MB
			MaxBodyBytes:      1 << 20,                    // 1 MB
		},
	}
}

// Default returns the effective default configuration, with any
// downstream build-time overrides (from defaultOverrides) merged
// on top of the base defaults.
func Default() *StaticConfig {
	base := BaseDefault()
	overrides := defaultOverrides()
	merged := mergeConfig(*base, overrides)
	return &merged
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
