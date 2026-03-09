package mcplog

import (
	"context"
	"fmt"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/stretchr/testify/suite"
)

type K8sErrorSuite struct {
	suite.Suite
}

func (s *K8sErrorSuite) TestClassifyK8sError() {
	gr := schema.GroupResource{Group: "", Resource: "pods"}

	s.Run("nil error returns false", func() {
		_, _, ok := classifyK8sError(nil, "any operation")
		s.False(ok)
	})

	s.Run("NotFound returns info level", func() {
		level, message, ok := classifyK8sError(apierrors.NewNotFound(gr, "test-pod"), "pod access")
		s.True(ok)
		s.Equal(LevelInfo, level)
		s.Contains(message, "Resource not found")
	})

	s.Run("Forbidden returns error level with operation", func() {
		level, message, ok := classifyK8sError(apierrors.NewForbidden(gr, "test-pod", nil), "pod access")
		s.True(ok)
		s.Equal(LevelError, level)
		s.Contains(message, "Permission denied")
		s.Contains(message, "pod access")
	})

	s.Run("Unauthorized returns error level", func() {
		level, message, ok := classifyK8sError(apierrors.NewUnauthorized("unauthorized"), "resource access")
		s.True(ok)
		s.Equal(LevelError, level)
		s.Contains(message, "Authentication failed")
	})

	s.Run("AlreadyExists returns warning level", func() {
		level, message, ok := classifyK8sError(apierrors.NewAlreadyExists(gr, "test-pod"), "resource creation")
		s.True(ok)
		s.Equal(LevelWarning, level)
		s.Contains(message, "already exists")
	})

	s.Run("Invalid returns error level", func() {
		level, message, ok := classifyK8sError(apierrors.NewInvalid(schema.GroupKind{Group: "", Kind: "Pod"}, "test-pod", nil), "resource update")
		s.True(ok)
		s.Equal(LevelError, level)
		s.Contains(message, "Invalid resource specification")
	})

	s.Run("BadRequest returns error level", func() {
		level, message, ok := classifyK8sError(apierrors.NewBadRequest("bad request"), "resource scaling")
		s.True(ok)
		s.Equal(LevelError, level)
		s.Contains(message, "Invalid request")
	})

	s.Run("Conflict returns error level", func() {
		level, message, ok := classifyK8sError(apierrors.NewConflict(gr, "test-pod", nil), "resource update")
		s.True(ok)
		s.Equal(LevelError, level)
		s.Contains(message, "Resource conflict")
	})

	s.Run("Timeout returns error level", func() {
		level, message, ok := classifyK8sError(apierrors.NewTimeoutError("timeout", 30), "node log access")
		s.True(ok)
		s.Equal(LevelError, level)
		s.Contains(message, "timeout")
	})

	s.Run("ServerTimeout returns error level", func() {
		level, message, ok := classifyK8sError(apierrors.NewServerTimeout(gr, "get", 60), "node stats access")
		s.True(ok)
		s.Equal(LevelError, level)
		s.Contains(message, "timeout")
	})

	s.Run("ServiceUnavailable returns error level", func() {
		level, message, ok := classifyK8sError(apierrors.NewServiceUnavailable("unavailable"), "events listing")
		s.True(ok)
		s.Equal(LevelError, level)
		s.Contains(message, "Service unavailable")
	})

	s.Run("TooManyRequests returns warning level", func() {
		level, message, ok := classifyK8sError(apierrors.NewTooManyRequests("rate limited", 10), "namespace listing")
		s.True(ok)
		s.Equal(LevelWarning, level)
		s.Contains(message, "Rate limited")
	})

	s.Run("other K8s API error returns error level", func() {
		level, message, ok := classifyK8sError(apierrors.NewInternalError(fmt.Errorf("internal error")), "resource access")
		s.True(ok)
		s.Equal(LevelError, level)
		s.Contains(message, "Operation failed")
	})
}

func (s *K8sErrorSuite) TestClassifyK8sErrorIgnoresNonK8sErrors() {
	s.Run("plain error returns false", func() {
		_, _, ok := classifyK8sError(fmt.Errorf("some non-k8s error"), "operation")
		s.False(ok)
	})

	s.Run("wrapped non-K8s error returns false", func() {
		inner := fmt.Errorf("connection refused")
		_, _, ok := classifyK8sError(fmt.Errorf("failed to connect: %w", inner), "operation")
		s.False(ok)
	})
}

func (s *K8sErrorSuite) TestClassifyK8sErrorWithWrappedK8sErrors() {
	gr := schema.GroupResource{Group: "", Resource: "secrets"}

	s.Run("wrapped NotFound is detected", func() {
		inner := apierrors.NewNotFound(gr, "my-secret")
		wrapped := fmt.Errorf("helm operation failed: %w", inner)
		level, message, ok := classifyK8sError(wrapped, "helm install")
		s.True(ok)
		s.Equal(LevelInfo, level)
		s.Contains(message, "Resource not found")
	})

	s.Run("wrapped Forbidden is detected", func() {
		inner := apierrors.NewForbidden(gr, "my-secret", nil)
		wrapped := fmt.Errorf("helm operation failed: %w", inner)
		level, message, ok := classifyK8sError(wrapped, "helm install")
		s.True(ok)
		s.Equal(LevelError, level)
		s.Contains(message, "Permission denied")
		s.Contains(message, "helm install")
	})

	s.Run("wrapped generic K8s API error is detected", func() {
		inner := apierrors.NewInternalError(fmt.Errorf("internal"))
		wrapped := fmt.Errorf("helm operation failed: %w", inner)
		level, message, ok := classifyK8sError(wrapped, "helm install")
		s.True(ok)
		s.Equal(LevelError, level)
		s.Contains(message, "Operation failed")
	})
}

func (s *K8sErrorSuite) TestHandleK8sErrorDoesNotPanic() {
	ctx := context.Background()

	s.Run("nil error", func() {
		s.NotPanics(func() {
			HandleK8sError(ctx, nil, "any operation")
		})
	})

	s.Run("K8s error without session in context", func() {
		s.NotPanics(func() {
			HandleK8sError(ctx, apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, "test"), "pod access")
		})
	})

	s.Run("non-K8s error without session in context", func() {
		s.NotPanics(func() {
			HandleK8sError(ctx, fmt.Errorf("some error"), "operation")
		})
	})
}

func TestK8sError(t *testing.T) {
	suite.Run(t, new(K8sErrorSuite))
}
