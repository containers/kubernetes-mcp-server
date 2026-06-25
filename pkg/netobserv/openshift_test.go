package netobserv

import (
	"net/http/httptest"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

type OpenShiftSuite struct {
	suite.Suite
}

func (s *OpenShiftSuite) TestClusterIsOpenShift() {
	s.Run("returns false without client", func() {
		s.False(clusterIsOpenShift(nil))
	})

	s.Run("returns false without discovery client", func() {
		s.False(isOpenShiftDiscovery(nil))
	})
}

func (s *OpenShiftSuite) TestIsOpenShiftDiscovery() {
	s.Run("returns true when project.openshift.io is registered", func() {
		srv := httptest.NewServer(test.NewInOpenShiftHandler())
		s.T().Cleanup(srv.Close)

		dc, err := discovery.NewDiscoveryClientForConfig(&rest.Config{Host: srv.URL})
		s.Require().NoError(err)
		s.True(isOpenShiftDiscovery(dc))
	})

	s.Run("returns false on plain Kubernetes discovery", func() {
		srv := httptest.NewServer(test.NewDiscoveryClientHandler())
		s.T().Cleanup(srv.Close)

		dc, err := discovery.NewDiscoveryClientForConfig(&rest.Config{Host: srv.URL})
		s.Require().NoError(err)
		s.False(isOpenShiftDiscovery(dc))
	})
}

func TestOpenShift(t *testing.T) {
	suite.Run(t, new(OpenShiftSuite))
}
