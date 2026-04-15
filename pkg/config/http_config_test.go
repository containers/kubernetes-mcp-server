package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type HTTPConfigSuite struct {
	suite.Suite
}

func (s *HTTPConfigSuite) TestDefaults() {
	cfg := Default()

	s.Run("sets read header timeout for Slowloris protection", func() {
		s.Equal(10*time.Second, cfg.HTTP.ReadHeaderTimeout.Duration())
	})

	s.Run("sets max body bytes to 16MB for large K8s manifests", func() {
		s.Equal(int64(16<<20), cfg.HTTP.MaxBodyBytes)
	})
}

func (s *HTTPConfigSuite) TestTOMLParsing() {
	s.Run("parses HTTP config fields", func() {
		tomlData := []byte(`
[http]
read_header_timeout = "5s"
max_body_bytes = 33554432
`)
		cfg, err := ReadToml(tomlData)
		s.Require().NoError(err)

		s.Equal(5*time.Second, cfg.HTTP.ReadHeaderTimeout.Duration())
		s.Equal(int64(32<<20), cfg.HTTP.MaxBodyBytes)
	})

	s.Run("uses defaults when not specified", func() {
		tomlData := []byte(`
log_level = 1
`)
		cfg, err := ReadToml(tomlData)
		s.Require().NoError(err)

		s.Equal(10*time.Second, cfg.HTTP.ReadHeaderTimeout.Duration())
		s.Equal(int64(16<<20), cfg.HTTP.MaxBodyBytes)
	})

	s.Run("partial config overrides only specified fields", func() {
		tomlData := []byte(`
[http]
max_body_bytes = 33554432
`)
		cfg, err := ReadToml(tomlData)
		s.Require().NoError(err)

		// Overridden value
		s.Equal(int64(32<<20), cfg.HTTP.MaxBodyBytes)

		// Default preserved
		s.Equal(10*time.Second, cfg.HTTP.ReadHeaderTimeout.Duration())
	})

	s.Run("returns error for invalid duration format", func() {
		tomlData := []byte(`
[http]
read_header_timeout = "invalid"
`)
		_, err := ReadToml(tomlData)
		s.Error(err)
	})
}

func (s *HTTPConfigSuite) TestValidate() {
	s.Run("zero RPS is valid (disabled)", func() {
		cfg := HTTPConfig{RateLimitRPS: 0}
		s.NoError(cfg.Validate())
	})

	s.Run("positive RPS is valid", func() {
		cfg := HTTPConfig{RateLimitRPS: 10}
		s.NoError(cfg.Validate())
	})

	s.Run("negative RPS is rejected", func() {
		cfg := HTTPConfig{RateLimitRPS: -1}
		err := cfg.Validate()
		s.Error(err)
		s.Contains(err.Error(), "rate_limit_rps must not be negative")
	})

	s.Run("negative burst is rejected", func() {
		cfg := HTTPConfig{RateLimitBurst: -5}
		err := cfg.Validate()
		s.Error(err)
		s.Contains(err.Error(), "rate_limit_burst must not be negative")
	})

	s.Run("zero burst is valid (uses default)", func() {
		cfg := HTTPConfig{RateLimitRPS: 10, RateLimitBurst: 0}
		s.NoError(cfg.Validate())
	})
}

func (s *HTTPConfigSuite) TestDefaultRateLimitBurst() {
	s.Run("constant has expected value", func() {
		s.Equal(10, DefaultRateLimitBurst)
	})
}

func TestHTTPConfig(t *testing.T) {
	suite.Run(t, new(HTTPConfigSuite))
}
