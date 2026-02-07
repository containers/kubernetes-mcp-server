package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/suite"
)

type ConfigSuite struct {
	suite.Suite
}

func (s *ConfigSuite) TestLoadFromEnv() {
	s.Run("defaults to disabled when env var not set", func() {
		os.Unsetenv("FUSION_TOOLS_ENABLED")
		cfg := LoadFromEnv()
		s.False(cfg.Enabled, "Fusion tools should be disabled by default")
	})

	s.Run("enables when env var is true", func() {
		os.Setenv("FUSION_TOOLS_ENABLED", "true")
		defer os.Unsetenv("FUSION_TOOLS_ENABLED")
		cfg := LoadFromEnv()
		s.True(cfg.Enabled, "Fusion tools should be enabled when FUSION_TOOLS_ENABLED=true")
	})

	s.Run("enables when env var is 1", func() {
		os.Setenv("FUSION_TOOLS_ENABLED", "1")
		defer os.Unsetenv("FUSION_TOOLS_ENABLED")
		cfg := LoadFromEnv()
		s.True(cfg.Enabled, "Fusion tools should be enabled when FUSION_TOOLS_ENABLED=1")
	})

	s.Run("disables when env var is false", func() {
		os.Setenv("FUSION_TOOLS_ENABLED", "false")
		defer os.Unsetenv("FUSION_TOOLS_ENABLED")
		cfg := LoadFromEnv()
		s.False(cfg.Enabled, "Fusion tools should be disabled when FUSION_TOOLS_ENABLED=false")
	})

	s.Run("disables when env var is 0", func() {
		os.Setenv("FUSION_TOOLS_ENABLED", "0")
		defer os.Unsetenv("FUSION_TOOLS_ENABLED")
		cfg := LoadFromEnv()
		s.False(cfg.Enabled, "Fusion tools should be disabled when FUSION_TOOLS_ENABLED=0")
	})

	s.Run("handles invalid env var gracefully", func() {
		os.Setenv("FUSION_TOOLS_ENABLED", "invalid")
		defer os.Unsetenv("FUSION_TOOLS_ENABLED")
		cfg := LoadFromEnv()
		s.False(cfg.Enabled, "Fusion tools should be disabled when FUSION_TOOLS_ENABLED has invalid value")
	})

	s.Run("handles whitespace in env var", func() {
		os.Setenv("FUSION_TOOLS_ENABLED", "  true  ")
		defer os.Unsetenv("FUSION_TOOLS_ENABLED")
		cfg := LoadFromEnv()
		s.True(cfg.Enabled, "Fusion tools should handle whitespace in env var")
	})
}

func TestConfigSuite(t *testing.T) {
	suite.Run(t, new(ConfigSuite))
}

// Made with Bob
