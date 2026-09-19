package kiali

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type OpenShiftSuite struct {
	suite.Suite
}

type mockFilteringProvider struct {
	hasGVKs  bool
	lastGVKs []schema.GroupVersionKind
}

func (m *mockFilteringProvider) IsTargetCompatibilityToolFiltersEnabled() bool { return true }

func (m *mockFilteringProvider) AnyTargetHasGVKs(_ context.Context, gvks []schema.GroupVersionKind) bool {
	m.lastGVKs = gvks
	return m.hasGVKs
}

func (s *OpenShiftSuite) TestIsOpenShiftFromProvider() {
	s.Run("returns false without provider", func() {
		s.False(IsOpenShiftFromProvider(context.Background(), nil))
	})

	s.Run("delegates to FilteringProvider", func() {
		provider := &mockFilteringProvider{hasGVKs: true}
		s.True(IsOpenShiftFromProvider(context.Background(), provider))
		s.Equal(openshiftProjectGVKs, provider.lastGVKs)
	})
}

func TestOpenShift(t *testing.T) {
	suite.Run(t, new(OpenShiftSuite))
}
