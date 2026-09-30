package sanitize_test

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/sanitize"
	"github.com/stretchr/testify/suite"
)

type SanitizerSuite struct {
	suite.Suite
}

func (s *SanitizerSuite) TestText() {
	s.Run("redacts escaped JSON string values", func() {
		actual := sanitize.Text(`{"password":"abc\"def","namespace":"default"}`)
		s.Equal(`{"password":"[REDACTED]","namespace":"default"}`, actual)
	})
}

func (s *SanitizerSuite) TestLog() {
	s.Run("preserves non-PEM headers", func() {
		input := "# -----BEGIN CONFIGURATION-----\nimportant line\nmore output"
		actual, truncated := sanitize.Log(input)
		s.Equal(input, actual)
		s.False(truncated)
	})

	s.Run("redacts a complete private key block", func() {
		input := "before\n-----BEGIN PRIVATE KEY-----\nprivate-material\n-----END PRIVATE KEY-----\nafter"
		actual, truncated := sanitize.Log(input)
		s.Equal("before\n[REDACTED PEM BLOCK]\nafter", actual)
		s.True(truncated)
	})

	s.Run("redacts a prefixed private key block", func() {
		input := "log: -----BEGIN PRIVATE KEY-----\nprivate-material\nlog: -----END PRIVATE KEY-----\nafter"
		actual, truncated := sanitize.Log(input)
		s.Equal("log: [REDACTED PEM BLOCK]\nafter", actual)
		s.True(truncated)
	})

	s.Run("marks an unterminated private key block truncated", func() {
		actual, truncated := sanitize.Log("before\n-----BEGIN OPENSSH PRIVATE KEY-----\nprivate-material")
		s.Equal("before\n[REDACTED PEM BLOCK]", actual)
		s.True(truncated)
	})

	s.Run("redacts only credential value tokens", func() {
		actual, truncated := sanitize.Log("Error: secret: missing-diagnosis-secret not found\ntoken=first-secret request failed")
		s.Equal("Error: secret: [REDACTED] not found\ntoken=[REDACTED] request failed", actual)
		s.False(truncated)
	})

	s.Run("redacts every credential on a line", func() {
		actual, _ := sanitize.Log("secret=first-secret token=second-secret complete")
		s.Equal("secret=[REDACTED] token=[REDACTED] complete", actual)
	})

	s.Run("redacts additional credential key names", func() {
		tests := []struct {
			name  string
			input string
			want  string
		}{
			{"client secret", "client_secret: client-value failed", "client_secret: [REDACTED] failed"},
			{"access key", "access_key=access-value failed", "access_key=[REDACTED] failed"},
			{"private key", "private_key: private-value failed", "private_key: [REDACTED] failed"},
		}
		for _, test := range tests {
			s.Run(test.name, func() {
				actual, _ := sanitize.Log(test.input)
				s.Equal(test.want, actual)
			})
		}
	})

	s.Run("preserves user flags", func() {
		input := "Deploying with --user 1000 and mounting /workspace"
		actual, _ := sanitize.Log(input)
		s.Equal(input, actual)
	})

	s.Run("redacts lowercase credential schemes", func() {
		actual, _ := sanitize.Log("request failed: bearer secret-token retrying")
		s.Equal("request failed: bearer [REDACTED] retrying", actual)
	})

	s.Run("redacts YAML secret blocks", func() {
		input := "password: |-\n  first secret line\n  second secret line\nstatus: failed"
		actual, truncated := sanitize.Log(input)
		s.Equal("password: [REDACTED]\nstatus: failed", actual)
		s.True(truncated)
	})

	s.Run("handles Unicode before a credential", func() {
		actual, _ := sanitize.Log("Ⱥtoken=not-a-real-secret complete")
		s.Equal("Ⱥtoken=[REDACTED] complete", actual)
	})
}

func TestSanitizer(t *testing.T) {
	suite.Run(t, new(SanitizerSuite))
}
