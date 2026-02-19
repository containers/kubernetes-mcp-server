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

func (s *K8sErrorSuite) TestHandleK8sError() {
	ctx := context.Background()
	gr := schema.GroupResource{Group: "", Resource: "pods"}

	s.Run("nil error is a no-op", func() {
		s.NotPanics(func() {
			HandleK8sError(ctx, nil, "any operation")
		})
	})

	s.Run("NotFound is handled", func() {
		s.NotPanics(func() {
			HandleK8sError(ctx, apierrors.NewNotFound(gr, "test-pod"), "pod access")
		})
	})

	s.Run("Forbidden is handled", func() {
		s.NotPanics(func() {
			HandleK8sError(ctx, apierrors.NewForbidden(gr, "test-pod", nil), "pod access")
		})
	})

	s.Run("Unauthorized is handled", func() {
		s.NotPanics(func() {
			HandleK8sError(ctx, apierrors.NewUnauthorized("unauthorized"), "resource access")
		})
	})

	s.Run("AlreadyExists is handled", func() {
		s.NotPanics(func() {
			HandleK8sError(ctx, apierrors.NewAlreadyExists(gr, "test-pod"), "resource creation")
		})
	})

	s.Run("Invalid is handled", func() {
		s.NotPanics(func() {
			HandleK8sError(ctx, apierrors.NewInvalid(schema.GroupKind{Group: "", Kind: "Pod"}, "test-pod", nil), "resource update")
		})
	})

	s.Run("BadRequest is handled", func() {
		s.NotPanics(func() {
			HandleK8sError(ctx, apierrors.NewBadRequest("bad request"), "resource scaling")
		})
	})

	s.Run("Conflict is handled", func() {
		s.NotPanics(func() {
			HandleK8sError(ctx, apierrors.NewConflict(gr, "test-pod", nil), "resource update")
		})
	})

	s.Run("Timeout is handled", func() {
		s.NotPanics(func() {
			HandleK8sError(ctx, apierrors.NewTimeoutError("timeout", 30), "node log access")
		})
	})

	s.Run("ServerTimeout is handled", func() {
		s.NotPanics(func() {
			HandleK8sError(ctx, apierrors.NewServerTimeout(gr, "get", 60), "node stats access")
		})
	})

	s.Run("ServiceUnavailable is handled", func() {
		s.NotPanics(func() {
			HandleK8sError(ctx, apierrors.NewServiceUnavailable("unavailable"), "events listing")
		})
	})

	s.Run("TooManyRequests is handled", func() {
		s.NotPanics(func() {
			HandleK8sError(ctx, apierrors.NewTooManyRequests("rate limited", 10), "namespace listing")
		})
	})

	s.Run("other K8s API error is handled", func() {
		s.NotPanics(func() {
			HandleK8sError(ctx, apierrors.NewInternalError(fmt.Errorf("internal error")), "resource access")
		})
	})
}

func (s *K8sErrorSuite) TestHandleK8sErrorIgnoresNonK8sErrors() {
	ctx := context.Background()

	s.Run("plain error is ignored", func() {
		s.NotPanics(func() {
			HandleK8sError(ctx, fmt.Errorf("some non-k8s error"), "operation")
		})
	})

	s.Run("wrapped non-K8s error is ignored", func() {
		inner := fmt.Errorf("connection refused")
		s.NotPanics(func() {
			HandleK8sError(ctx, fmt.Errorf("failed to connect: %w", inner), "operation")
		})
	})
}

func (s *K8sErrorSuite) TestHandleK8sErrorWithWrappedK8sErrors() {
	ctx := context.Background()
	gr := schema.GroupResource{Group: "", Resource: "secrets"}

	s.Run("wrapped NotFound is detected", func() {
		inner := apierrors.NewNotFound(gr, "my-secret")
		wrapped := fmt.Errorf("helm operation failed: %w", inner)
		s.NotPanics(func() {
			HandleK8sError(ctx, wrapped, "helm install")
		})
	})

	s.Run("wrapped Forbidden is detected", func() {
		inner := apierrors.NewForbidden(gr, "my-secret", nil)
		wrapped := fmt.Errorf("helm operation failed: %w", inner)
		s.NotPanics(func() {
			HandleK8sError(ctx, wrapped, "helm install")
		})
	})

	s.Run("wrapped generic K8s API error is detected", func() {
		inner := apierrors.NewInternalError(fmt.Errorf("internal"))
		wrapped := fmt.Errorf("helm operation failed: %w", inner)
		s.NotPanics(func() {
			HandleK8sError(ctx, wrapped, "helm install")
		})
	})
}

func TestK8sError(t *testing.T) {
	suite.Run(t, new(K8sErrorSuite))
}
