package api

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery/cached/memory"
)

type NotFoundTestSuite struct {
	suite.Suite
}

func (s *NotFoundTestSuite) TestIsNotFound() {
	notFound := apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, "missing")
	s.Run("recognizes Kubernetes and cached discovery not-found errors", func() {
		s.True(IsNotFound(notFound))
		s.True(IsNotFound(memory.ErrCacheNotFound))
		s.True(IsNotFound(fmt.Errorf("discovery: %w", memory.ErrCacheNotFound)))
	})
	s.Run("does not classify uncertain discovery failures as absent", func() {
		s.False(IsNotFound(nil))
		s.False(IsNotFound(errors.New("discovery unavailable")))
		s.False(IsNotFound(errors.Join(notFound, errors.New("discovery unavailable"))))
		s.False(IsNotFound(fmt.Errorf("targets: %w", errors.Join(memory.ErrCacheNotFound, errors.New("discovery unavailable")))))
	})
	s.Run("recognizes not-found across all targets", func() {
		s.True(IsNotFound(errors.Join(notFound, memory.ErrCacheNotFound)))
	})
}

func TestIsNotFound(t *testing.T) {
	suite.Run(t, new(NotFoundTestSuite))
}
