package config

import (
	"os"
	"strconv"
)

// ValidationConfig contains pre-execution validation configuration.
// When enabled, validates tool calls before execution
type ValidationConfig struct {
	// Defaults to false.
	Enabled *bool `toml:"enabled,omitempty"`
}

// IsEnabled returns true if validation is enabled.
// Environment variable MCP_VALIDATION_ENABLED takes precedence over config.
// Defaults to false.
func (c *ValidationConfig) IsEnabled() bool {
	if v := os.Getenv("MCP_VALIDATION_ENABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	if c.Enabled != nil {
		return *c.Enabled
	}
	return false
}
