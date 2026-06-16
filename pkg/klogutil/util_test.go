package klogutil

import (
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/stretchr/testify/suite"
)

type KlogUtilSuite struct {
	suite.Suite
}

func TestKlogUtil(t *testing.T) {
	suite.Run(t, new(KlogUtilSuite))
}

// newCapturingLogger returns a logr.Logger that appends each rendered log line
// to the provided slice, making it easy to assert on actual log output.
func newCapturingLogger(lines *[]string, opts funcr.Options) logr.Logger {
	return funcr.New(func(prefix, args string) {
		*lines = append(*lines, prefix+args)
	}, opts)
}

func (s *KlogUtilSuite) TestErr() {
	s.Run("constructs exception.message attribute", func() {
		attr := Err(errors.New("connection refused"))
		s.Equal("exception.message", attr.K)
		s.Equal("connection refused", attr.V)
	})

	s.Run("handles nil error without panicking", func() {
		var attr Attr
		s.Require().NotPanics(func() { attr = Err(nil) })
		s.Equal("exception.message", attr.K)
		s.Equal("", attr.V)
	})
}

func (s *KlogUtilSuite) TestField() {
	s.Run("constructs arbitrary key/value attribute", func() {
		attr := Field("validator_name", "rbac")
		s.Equal("validator_name", attr.K)
		s.Equal("rbac", attr.V)
	})
}

func (s *KlogUtilSuite) TestLogInfo() {
	s.Run("renders field and error as real attributes", func() {
		var lines []string
		logger := newCapturingLogger(&lines, funcr.Options{})

		LogInfo(logger, "RBAC pre-validation failed",
			Field("validator_name", "rbac"), Err(errors.New("connection refused")))

		s.Require().Len(lines, 1)
		s.Contains(lines[0], "RBAC pre-validation failed")
		s.Contains(lines[0], `"validator_name"="rbac"`)
		s.Contains(lines[0], `"exception.message"="connection refused"`)
	})

	s.Run("does not produce a non-string key (regression for reverted WithError)", func() {
		var lines []string
		logger := newCapturingLogger(&lines, funcr.Options{})

		LogInfo(logger, "boom", Err(errors.New("connection refused")))

		s.Require().Len(lines, 1)
		s.Contains(lines[0], `"exception.message"="connection refused"`)
		s.NotContains(lines[0], "non-string-key")
		s.NotContains(lines[0], "<no-value>")
	})

	s.Run("renders non-string attribute values", func() {
		var lines []string
		logger := newCapturingLogger(&lines, funcr.Options{})

		LogInfo(logger, "typed values", Field("config.default_value", 1.0), Field("attempt", 3))

		s.Require().Len(lines, 1)
		s.Contains(lines[0], `"config.default_value"=1`)
		s.Contains(lines[0], `"attempt"=3`)
	})

	s.Run("works with no attributes", func() {
		var lines []string
		logger := newCapturingLogger(&lines, funcr.Options{})

		LogInfo(logger, "bare message")

		s.Require().Len(lines, 1)
		s.Contains(lines[0], "bare message")
	})

	s.Run("respects .V(n) verbosity gating", func() {
		var lines []string
		logger := newCapturingLogger(&lines, funcr.Options{Verbosity: 3})

		LogInfo(logger.V(4), "too verbose", Field("k", "v"))
		s.Empty(lines, "a V(4) call must emit nothing when verbosity is 3")

		LogInfo(logger.V(3), "visible", Field("k", "v"))
		s.Require().Len(lines, 1)
		s.Contains(lines[0], "visible")
	})
}

func (s *KlogUtilSuite) TestLogWarn() {
	s.Run("renders WARN severity with field and error", func() {
		var lines []string
		logger := newCapturingLogger(&lines, funcr.Options{})

		LogWarn(logger, "disk full",
			Field("path", "/var/log"), Err(errors.New("boom")))

		s.Require().Len(lines, 1)
		s.Contains(lines[0], "disk full")
		s.Contains(lines[0], `"log.severity"="WARN"`)
		s.Contains(lines[0], `"path"="/var/log"`)
		s.Contains(lines[0], `"exception.message"="boom"`)
	})

	s.Run("carries WARN severity with no extra attributes", func() {
		var lines []string
		logger := newCapturingLogger(&lines, funcr.Options{})

		LogWarn(logger, "bare warning")

		s.Require().Len(lines, 1)
		s.Contains(lines[0], "bare warning")
		s.Contains(lines[0], `"log.severity"="WARN"`)
	})

	s.Run("respects .V(n) verbosity gating", func() {
		var lines []string
		logger := newCapturingLogger(&lines, funcr.Options{Verbosity: 3})

		LogWarn(logger.V(4), "too verbose")
		s.Empty(lines, "a V(4) warn must emit nothing when verbosity is 3")

		LogWarn(logger.V(3), "visible warning")
		s.Require().Len(lines, 1)
		s.Contains(lines[0], "visible warning")
		s.Contains(lines[0], `"log.severity"="WARN"`)
	})
}
