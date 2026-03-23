package config

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type TLSSuite struct {
	suite.Suite
}

func (s *TLSSuite) TestValidateURLRequiresTLS() {
	s.Run("returns nil for empty URL", func() {
		err := ValidateURLRequiresTLS("", "test_url")
		s.NoError(err)
	})

	s.Run("returns nil for HTTPS URL", func() {
		err := ValidateURLRequiresTLS("https://example.com/path", "test_url")
		s.NoError(err)
	})

	s.Run("returns error for HTTP URL", func() {
		err := ValidateURLRequiresTLS("http://example.com/path", "test_url")
		s.Require().Error(err)
		s.Contains(err.Error(), "require_tls is enabled but test_url uses \"http\" scheme (HTTPS required)")
	})

	s.Run("returns error for non-HTTPS scheme", func() {
		err := ValidateURLRequiresTLS("ftp://example.com/path", "test_url")
		s.Require().Error(err)
		s.Contains(err.Error(), "uses \"ftp\" scheme (HTTPS required)")
	})

	s.Run("includes field name in error message", func() {
		err := ValidateURLRequiresTLS("http://example.com", "my_custom_field")
		s.Require().Error(err)
		s.Contains(err.Error(), "my_custom_field")
	})

	s.Run("returns error for invalid URL", func() {
		err := ValidateURLRequiresTLS("://invalid", "test_url")
		s.Require().Error(err)
		s.Contains(err.Error(), "invalid test_url")
	})
}

func TestTLS(t *testing.T) {
	suite.Run(t, new(TLSSuite))
}
