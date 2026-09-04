package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type CAURLConfigSuite struct {
	BaseConfigSuite
}

func TestCAURLConfigSuite(t *testing.T) {
	suite.Run(t, new(CAURLConfigSuite))
}

func (s *CAURLConfigSuite) TestDefaultRefreshInterval() {
	s.Run("refreshes cached CAs daily by default", func() {
		s.Equal(Duration(24*time.Hour), Default().CARefreshInterval)
	})
	s.Run("defaults cache dir to empty meaning system temp dir", func() {
		s.Empty(Default().CACacheDir)
	})
}

func (s *CAURLConfigSuite) TestCAURLConfigFromTOML() {
	s.Run("parses ca_cache_dir and ca_refresh_interval", func() {
		path := s.writeConfig(`
ca_cache_dir = "/var/cache/mcp-ca"
ca_refresh_interval = "30m"
`)
		cfg, err := Read(s.T().Context(), path, "")
		s.Require().NoError(err)
		s.Equal("/var/cache/mcp-ca", cfg.GetCACacheDir())
		s.Equal(30*time.Minute, cfg.GetCARefreshInterval())
	})
	s.Run("zero refresh interval disables refresh", func() {
		path := s.writeConfig(`ca_refresh_interval = "0s"`)
		cfg, err := Read(s.T().Context(), path, "")
		s.Require().NoError(err)
		s.Zero(cfg.GetCARefreshInterval())
	})
}
