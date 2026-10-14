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
	s.Run("redacts SSH keys and credentials in key=value form", func() {
		out := Secrets("ssh-ed25519 AAAA host\nexport PASSWORD=hunter2 token=abc")
		s.Equal("ssh-ed25519 <REDACTED>\nexport PASSWORD=<REDACTED>", out)
	})
	s.Run("redacts the whole body of a PEM block, not just its header", func() {
		out := Secrets("before\n-----BEGIN RSA PRIVATE KEY-----\nMIIEow\nAAAA\n-----END RSA PRIVATE KEY-----\nafter")
		s.Equal("before\n-----BEGIN<REDACTED>\n<REDACTED>\n-----END RSA PRIVATE KEY-----\nafter", out)
	})
	s.Run("an unterminated PEM block redacts to the end of the text", func() {
		s.Equal("-----BEGIN<REDACTED>\n<REDACTED>", Secrets("-----BEGIN CERTIFICATE-----\nMIIEow\nAAAA"))
	})
	s.Run("leaves clean text untouched", func() {
		s.Equal("a\nb\n", Secrets("a\nb\n"))
	})
}

func TestRedact(t *testing.T) {
	suite.Run(t, new(RedactSuite))
}
