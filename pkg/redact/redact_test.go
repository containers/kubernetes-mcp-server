package redact

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type RedactSuite struct {
	suite.Suite
}

func (s *RedactSuite) TestSecrets() {
	s.Run("redacts the rest of a line after a marker, case-insensitively", func() {
		s.Equal("Authorization: Bearer <REDACTED>\nok\ncurl -u <REDACTED>", Secrets("Authorization: Bearer abc.def\nok\ncurl -u admin:pw http://x"))
	})
	s.Run("redacts PEM, SSH keys and credentials in key=value form", func() {
		out := Secrets("-----BEGIN RSA PRIVATE KEY-----\nssh-ed25519 AAAA host\nexport PASSWORD=hunter2 token=abc")
		s.Equal("-----BEGIN<REDACTED>\nssh-ed25519 <REDACTED>\nexport PASSWORD=<REDACTED>", out)
	})
	s.Run("leaves clean text untouched", func() {
		s.Equal("a\nb\n", Secrets("a\nb\n"))
	})
}

func TestRedact(t *testing.T) {
	suite.Run(t, new(RedactSuite))
}
