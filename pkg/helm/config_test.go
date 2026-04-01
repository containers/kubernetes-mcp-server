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
	s.Run("valid clean config", func() {
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
	s.Run("normalizes allowed registries to lowercase and trims trailing slashes", func() {
		cfg := &Config{
			AllowedRegistries: []string{
				"OCI://GHCR.IO/myorg/",
				"HTTPS://Charts.Example.COM/repo/",
			},
		}
		s.NoError(cfg.Validate())
		s.Equal("oci://ghcr.io/myorg", cfg.AllowedRegistries[0])
		s.Equal("https://charts.example.com/repo", cfg.AllowedRegistries[1])
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
	s.Run("normalizes storage driver to lowercase", func() {
		cfg := &Config{
			StorageDriver: "COnfIgmAP",
		}
		s.NoError(cfg.Validate())
		s.Equal("configmap", cfg.StorageDriver)
	})
	s.Run("accepts secret storage driver", func() {
		cfg := &Config{
			StorageDriver: "secret",
		}
		s.NoError(cfg.Validate())
	})
	s.Run("accepts configmap storage driver", func() {
		cfg := &Config{
			StorageDriver: "configmap",
		}
		s.NoError(cfg.Validate())
	})
	s.Run("rejects unsupported memory storage driver", func() {
		cfg := &Config{
			StorageDriver: "memory",
		}
		err := cfg.Validate()
		s.Error(err)
		s.Contains(err.Error(), "unsupported Helm storage driver")
	})
	s.Run("rejects unsupported sql storage driver", func() {
		cfg := &Config{
			StorageDriver: "sql",
		}
		err := cfg.Validate()
		s.Error(err)
		s.Contains(err.Error(), "unsupported Helm storage driver")
	})
	s.Run("rejects arbitrary storage string", func() {
		cfg := &Config{
			StorageDriver: "random",
		}
		err := cfg.Validate()
		s.Error(err)
		s.Contains(err.Error(), "unsupported Helm storage driver")
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
	s.Run("parses storage_driver from TOML", func() {
		cfg := test.Must(config.ReadToml([]byte(`
			[toolset_configs.helm]
			storage_driver = "configmap"
		`)))
		helmCfg, ok := cfg.GetToolsetConfig("helm")
		s.Require().True(ok)
		hc, ok := helmCfg.(*Config)
		s.Require().True(ok)
		s.Equal("configmap", hc.StorageDriver)
	})
	s.Run("rejects unsupported storage_driver in TOML", func() {
		_, err := config.ReadToml([]byte(`
			[toolset_configs.helm]
			storage_driver = "memory"
		`))
		s.Error(err)
		s.Contains(err.Error(), "unsupported Helm storage driver")
	})
}

func TestConfig(t *testing.T) {
	suite.Run(t, new(ConfigSuite))
}
