package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type CAURLConfigSuite struct {
	suite.Suite
}

func TestCAURLConfigSuite(t *testing.T) {
	suite.Run(t, new(CAURLConfigSuite))
}

func (s *CAURLConfigSuite) TestDefaultRefreshInterval() {
	s.Run("refreshes cached CAs daily by default", func() {
		s.Equal(24*time.Hour, New().CARefreshInterval.Get())
	})
}

func (s *CAURLConfigSuite) TestCAURLConfigFromTOML() {
	s.Run("parses ca_refresh_interval", func() {
		cfg, err := ReadToml(s.T().Context(), []byte(`ca_refresh_interval = "30m"`), WithBaseDefault())
		s.Require().NoError(err)
		s.Equal(30*time.Minute, cfg.CARefreshInterval.Get())
	})
	s.Run("zero refresh interval disables refresh", func() {
		cfg, err := ReadToml(s.T().Context(), []byte(`ca_refresh_interval = "0s"`), WithBaseDefault())
		s.Require().NoError(err)
		s.Zero(cfg.CARefreshInterval.Get())
	})
}
