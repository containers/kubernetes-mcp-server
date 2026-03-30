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

	s.Run("sets read timeout", func() {
		s.Equal(30*time.Second, cfg.HTTP.ReadTimeout.Duration())
	})

	s.Run("sets idle timeout per Apache recommendation", func() {
		s.Equal(60*time.Second, cfg.HTTP.IdleTimeout.Duration())
	})

	s.Run("sets read header timeout for Slowloris protection", func() {
		s.Equal(10*time.Second, cfg.HTTP.ReadHeaderTimeout.Duration())
	})

	s.Run("sets max header bytes to 1MB", func() {
		s.Equal(1<<20, cfg.HTTP.MaxHeaderBytes)
	})

	s.Run("sets max body bytes to 1MB", func() {
		s.Equal(int64(1<<20), cfg.HTTP.MaxBodyBytes)
	})
}

func (s *HTTPConfigSuite) TestTOMLParsing() {
	s.Run("parses all HTTP config fields", func() {
		tomlData := []byte(`
[http]
read_timeout = "15s"
idle_timeout = "45s"
read_header_timeout = "5s"
max_header_bytes = 2097152
max_body_bytes = 5242880
`)
		cfg, err := ReadToml(tomlData)
		s.Require().NoError(err)

		s.Equal(15*time.Second, cfg.HTTP.ReadTimeout.Duration())
		s.Equal(45*time.Second, cfg.HTTP.IdleTimeout.Duration())
		s.Equal(5*time.Second, cfg.HTTP.ReadHeaderTimeout.Duration())
		s.Equal(2<<20, cfg.HTTP.MaxHeaderBytes)
		s.Equal(int64(5<<20), cfg.HTTP.MaxBodyBytes)
	})

	s.Run("uses defaults when not specified", func() {
		tomlData := []byte(`
log_level = 1
`)
		cfg, err := ReadToml(tomlData)
		s.Require().NoError(err)

		s.Equal(30*time.Second, cfg.HTTP.ReadTimeout.Duration())
		s.Equal(60*time.Second, cfg.HTTP.IdleTimeout.Duration())
		s.Equal(10*time.Second, cfg.HTTP.ReadHeaderTimeout.Duration())
		s.Equal(1<<20, cfg.HTTP.MaxHeaderBytes)
		s.Equal(int64(1<<20), cfg.HTTP.MaxBodyBytes)
	})

	s.Run("partial config overrides only specified fields", func() {
		tomlData := []byte(`
[http]
read_timeout = "45s"
max_body_bytes = 20971520
`)
		cfg, err := ReadToml(tomlData)
		s.Require().NoError(err)

		// Overridden values
		s.Equal(45*time.Second, cfg.HTTP.ReadTimeout.Duration())
		s.Equal(int64(20<<20), cfg.HTTP.MaxBodyBytes)

		// Default values preserved
		s.Equal(60*time.Second, cfg.HTTP.IdleTimeout.Duration())
		s.Equal(10*time.Second, cfg.HTTP.ReadHeaderTimeout.Duration())
		s.Equal(1<<20, cfg.HTTP.MaxHeaderBytes)
	})

	s.Run("returns error for invalid duration format", func() {
		tomlData := []byte(`
[http]
read_timeout = "invalid"
`)
		_, err := ReadToml(tomlData)
		s.Error(err)
	})
}

func TestHTTPConfig(t *testing.T) {
	suite.Run(t, new(HTTPConfigSuite))
}
