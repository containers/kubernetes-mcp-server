package config

import (
	"os"
	"strconv"
	"strings"
)

// FusionConfig holds IBM Fusion-specific configuration
type FusionConfig struct {
	// Enabled controls whether Fusion tools are registered
	Enabled bool
}

// LoadFromEnv loads Fusion configuration from environment variables
func LoadFromEnv() *FusionConfig {
	cfg := &FusionConfig{
		Enabled: false,
	}

	// Check FUSION_TOOLS_ENABLED environment variable
	if val := strings.TrimSpace(os.Getenv("FUSION_TOOLS_ENABLED")); val != "" {
		enabled, err := strconv.ParseBool(val)
		if err == nil {
			cfg.Enabled = enabled
		}
	}

	return cfg
}

// Made with Bob
