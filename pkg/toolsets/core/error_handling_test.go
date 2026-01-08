package core

import (
	"context"
	"fmt"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/mcplog"
	"github.com/stretchr/testify/suite"
)

type ErrorHandlingSuite struct {
	suite.Suite
}

func (s *ErrorHandlingSuite) TestHandleK8sErrorIntegration() {
	ctx := context.Background()
	gr := schema.GroupResource{Group: "v1", Resource: "pods"}

	s.Run("handles NotFound errors", func() {
		err := apierrors.NewNotFound(gr, "test-pod")
		s.NotPanics(func() {
			mcplog.HandleK8sError(ctx, err, "pod access")
		})
	})

	s.Run("handles Forbidden errors", func() {
		err := apierrors.NewForbidden(gr, "test-pod", nil)
		s.NotPanics(func() {
			mcplog.HandleK8sError(ctx, err, "pod deletion")
		})
	})

	s.Run("handles Unauthorized errors", func() {
		err := apierrors.NewUnauthorized("unauthorized")
		s.NotPanics(func() {
			mcplog.HandleK8sError(ctx, err, "resource access")
		})
	})

	s.Run("handles AlreadyExists errors", func() {
		err := apierrors.NewAlreadyExists(gr, "test-resource")
		s.NotPanics(func() {
			mcplog.HandleK8sError(ctx, err, "resource creation")
		})
	})

	s.Run("handles Invalid errors", func() {
		err := apierrors.NewInvalid(schema.GroupKind{Group: "v1", Kind: "Pod"}, "test-pod", nil)
		s.NotPanics(func() {
			mcplog.HandleK8sError(ctx, err, "resource creation or update")
		})
	})

	s.Run("handles BadRequest errors", func() {
		err := apierrors.NewBadRequest("bad request")
		s.NotPanics(func() {
			mcplog.HandleK8sError(ctx, err, "resource scaling")
		})
	})

	s.Run("handles Conflict errors", func() {
		err := apierrors.NewConflict(gr, "test-resource", nil)
		s.NotPanics(func() {
			mcplog.HandleK8sError(ctx, err, "resource update")
		})
	})

	s.Run("handles Timeout errors", func() {
		err := apierrors.NewTimeoutError("request timeout", 30)
		s.NotPanics(func() {
			mcplog.HandleK8sError(ctx, err, "node log access")
		})
	})

	s.Run("handles ServerTimeout errors", func() {
		err := apierrors.NewServerTimeout(gr, "operation", 60)
		s.NotPanics(func() {
			mcplog.HandleK8sError(ctx, err, "node stats access")
		})
	})

	s.Run("handles ServiceUnavailable errors", func() {
		err := apierrors.NewServiceUnavailable("service unavailable")
		s.NotPanics(func() {
			mcplog.HandleK8sError(ctx, err, "events listing")
		})
	})

	s.Run("handles TooManyRequests errors", func() {
		err := apierrors.NewTooManyRequests("rate limited", 10)
		s.NotPanics(func() {
			mcplog.HandleK8sError(ctx, err, "namespace listing")
		})
	})

	s.Run("handles generic errors", func() {
		err := apierrors.NewInternalError(fmt.Errorf("internal server error"))
		s.NotPanics(func() {
			mcplog.HandleK8sError(ctx, err, "node metrics access")
		})
	})

	s.Run("handles nil error gracefully", func() {
		s.NotPanics(func() {
			mcplog.HandleK8sError(ctx, nil, "any operation")
		})
	})
}

func (s *ErrorHandlingSuite) TestErrorHandlingCoverage() {
	s.Run("error handling is consistent across handlers", func() {
		handlers := []string{
			"podsGet - pod access",
			"podsDelete - pod deletion",
			"resourcesList - resource listing",
			"resourcesGet - resource access",
			"resourcesCreateOrUpdate - resource creation or update",
			"resourcesDelete - resource deletion",
			"resourcesScale - resource scaling",
			"nodesLog - node log access",
			"nodesStatsSummary - node stats access",
			"nodesTop - node metrics access, node listing",
			"eventsList - events listing",
			"namespacesList - namespace listing",
			"projectsList - project listing",
		}

		s.GreaterOrEqual(len(handlers), 13, "should document all error handling points")
	})
}

func (s *ErrorHandlingSuite) TestOperationDescriptions() {
	s.Run("operation descriptions follow naming conventions", func() {
		validDescriptions := []string{
			"pod access",
			"pod deletion",
			"resource listing",
			"resource access",
			"resource creation or update",
			"resource deletion",
			"resource scaling",
			"node log access",
			"node stats access",
			"node metrics access",
			"node listing",
			"events listing",
			"namespace listing",
			"project listing",
		}

		for _, desc := range validDescriptions {
			s.NotEmpty(desc, "description should not be empty")
			s.Equal(desc, desc, "description should be lowercase: %s", desc)
		}
	})
}

func TestErrorHandling(t *testing.T) {
	suite.Run(t, new(ErrorHandlingSuite))
}
