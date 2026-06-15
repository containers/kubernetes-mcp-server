package klogutil

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/stretchr/testify/suite"
	"k8s.io/klog/v2"
)

type KlogUtilSuite struct {
	suite.Suite
}

func TestKlogUtil(t *testing.T) {
	suite.Run(t, new(KlogUtilSuite))
}

// newCapturingLogger returns a logr.Logger that appends each log line to the
// provided slice, making it easy to assert on log output.
func newCapturingLogger(lines *[]string) logr.Logger {
	return funcr.New(func(prefix, args string) {
		*lines = append(*lines, prefix+args)
	}, funcr.Options{})
}

func (s *KlogUtilSuite) TestWarn() {
	s.Run("logs message with WARN severity", func() {
		var lines []string
		logger := newCapturingLogger(&lines)
		ctx := klog.NewContext(context.Background(), logger)

		Warn(ctx, "something happened")

		s.Require().Len(lines, 1)
		s.Contains(lines[0], "something happened")
		s.Contains(lines[0], "log.severity")
		s.Contains(lines[0], "WARN")
	})

	s.Run("includes extra key-value pairs", func() {
		var lines []string
		logger := newCapturingLogger(&lines)
		ctx := klog.NewContext(context.Background(), logger)

		Warn(ctx, "disk full", "path", "/var/log")

		s.Require().Len(lines, 1)
		s.Contains(lines[0], "path")
		s.Contains(lines[0], "/var/log")
	})

	s.Run("works with no extra key-value pairs", func() {
		var lines []string
		logger := newCapturingLogger(&lines)
		ctx := klog.NewContext(context.Background(), logger)

		Warn(ctx, "bare warning")

		s.Require().Len(lines, 1)
		s.Contains(lines[0], "bare warning")
	})
}

func (s *KlogUtilSuite) TestWarnLogger() {
	s.Run("logs message with WARN severity", func() {
		var lines []string
		logger := newCapturingLogger(&lines)

		WarnLogger(logger, "timeout exceeded")

		s.Require().Len(lines, 1)
		s.Contains(lines[0], "timeout exceeded")
		s.Contains(lines[0], "log.severity")
		s.Contains(lines[0], "WARN")
	})

	s.Run("includes extra key-value pairs", func() {
		var lines []string
		logger := newCapturingLogger(&lines)

		WarnLogger(logger, "retry", "attempt", 3)

		s.Require().Len(lines, 1)
		s.Contains(lines[0], "attempt")
		s.Contains(lines[0], "3")
	})
}
