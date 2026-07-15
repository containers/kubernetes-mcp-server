package netobserv

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

type OpenShiftSuite struct {
	suite.Suite
}

func (s *OpenShiftSuite) TestIsOpenShiftFromProvider() {
	s.Run("returns false without provider", func() {
		s.False(isOpenShiftFromProvider(context.Background(), nil))
	})

	s.Run("delegates to FilteringProvider", func() {
		provider := &mockFilteringProvider{hasGVKs: true}
		s.True(isOpenShiftFromProvider(context.Background(), provider))
		s.Equal(openshiftProjectGVKs, provider.lastGVKs)
	})
}

func (s *OpenShiftSuite) TestClusterIsOpenShiftFromDiscovery() {
	s.Run("returns false without discovery client", func() {
		s.False(clusterIsOpenShiftFromDiscovery(nil))
	})

	s.Run("returns true when project.openshift.io is registered", func() {
		srv := httptest.NewServer(test.NewInOpenShiftHandler())
		s.T().Cleanup(srv.Close)

		dc, err := discovery.NewDiscoveryClientForConfig(&rest.Config{Host: srv.URL})
		s.Require().NoError(err)
		s.True(clusterIsOpenShiftFromDiscovery(dc))
	})

	s.Run("returns false on plain Kubernetes discovery", func() {
		srv := httptest.NewServer(test.NewDiscoveryClientHandler())
		s.T().Cleanup(srv.Close)

		dc, err := discovery.NewDiscoveryClientForConfig(&rest.Config{Host: srv.URL})
		s.Require().NoError(err)
		s.False(clusterIsOpenShiftFromDiscovery(dc))
	})
}

type mockFilteringProvider struct {
	hasGVKs  bool
	lastGVKs []schema.GroupVersionKind
}

func (m *mockFilteringProvider) IsTargetCompatibilityToolFiltersEnabled() bool {
	return true
}

func (m *mockFilteringProvider) AnyTargetHasGVKs(_ context.Context, gvks []schema.GroupVersionKind) bool {
	m.lastGVKs = gvks
	return m.hasGVKs
}

func TestOpenShift(t *testing.T) {
	suite.Run(t, new(OpenShiftSuite))
}
