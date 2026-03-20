package helm

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
)

type ConfigSuite struct {
	suite.Suite
}

func (s *ConfigSuite) TestValidate() {
	s.Run("valid config with no allowed registries", func() {
		cfg := &Config{}
		s.NoError(cfg.Validate())
	})
	s.Run("valid config with allowed registries", func() {
		cfg := &Config{
			AllowedRegistries: []string{
				"oci://ghcr.io/myorg",
				"https://charts.example.com",
			},
		}
		s.NoError(cfg.Validate())
	})
	s.Run("nil config returns error", func() {
		var cfg *Config
		s.Error(cfg.Validate())
	})
	s.Run("rejects entry without scheme", func() {
		cfg := &Config{AllowedRegistries: []string{"ghcr.io/myorg"}}
		err := cfg.Validate()
		s.Error(err)
		s.Contains(err.Error(), "must be a valid URL with scheme and host")
	})
	s.Run("rejects entry with http:// scheme", func() {
		cfg := &Config{AllowedRegistries: []string{"http://example.com"}}
		err := cfg.Validate()
		s.Error(err)
		s.Contains(err.Error(), "must use oci:// or https:// scheme")
	})
	s.Run("rejects entry with file:// scheme", func() {
		cfg := &Config{AllowedRegistries: []string{"file:///tmp"}}
		err := cfg.Validate()
		s.Error(err)
		s.Contains(err.Error(), "must use oci:// or https:// scheme")
	})
}

func (s *ConfigSuite) TestParser() {
	s.Run("parses allowed_registries from TOML", func() {
		cfg := test.Must(config.ReadToml([]byte(`
			[toolset_configs.helm]
			allowed_registries = ["oci://ghcr.io/myorg", "https://charts.example.com"]
		`)))
		helmCfg, ok := cfg.GetToolsetConfig("helm")
		s.Require().True(ok)
		hc, ok := helmCfg.(*Config)
		s.Require().True(ok)
		s.Equal([]string{"oci://ghcr.io/myorg", "https://charts.example.com"}, hc.AllowedRegistries)
	})
	s.Run("parses empty config from TOML", func() {
		cfg := test.Must(config.ReadToml([]byte(`
			[toolset_configs.helm]
		`)))
		helmCfg, ok := cfg.GetToolsetConfig("helm")
		s.Require().True(ok)
		hc, ok := helmCfg.(*Config)
		s.Require().True(ok)
		s.Empty(hc.AllowedRegistries)
	})
	s.Run("rejects invalid allowed_registries entry", func() {
		_, err := config.ReadToml([]byte(`
			[toolset_configs.helm]
			allowed_registries = ["not-a-url"]
		`))
		s.Error(err)
		s.Contains(err.Error(), "must be a valid URL with scheme and host")
	})
	s.Run("rejects http:// in allowed_registries", func() {
		_, err := config.ReadToml([]byte(`
			[toolset_configs.helm]
			allowed_registries = ["http://evil.example.com"]
		`))
		s.Error(err)
		s.Contains(err.Error(), "must use oci:// or https:// scheme")
	})
}

func TestConfig(t *testing.T) {
	suite.Run(t, new(ConfigSuite))
}
