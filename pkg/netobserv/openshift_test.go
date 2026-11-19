package netobserv

import (
	"net/http"
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

func (s *OpenShiftSuite) TestClusterIsOpenShiftFromDiscovery() {
	s.Run("uses safe defaults without discovery client", func() {
		s.True(clusterIsOpenShiftFromDiscovery(nil))
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

	s.Run("returns false when the OpenShift API group is absent", func() {
		srv := httptest.NewServer(http.NotFoundHandler())
		s.T().Cleanup(srv.Close)

		dc, err := discovery.NewDiscoveryClientForConfig(&rest.Config{Host: srv.URL})
		s.Require().NoError(err)
		s.False(clusterIsOpenShiftFromDiscovery(dc))
	})

	s.Run("uses safe defaults when discovery fails", func() {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "discovery unavailable", http.StatusInternalServerError)
		}))
		s.T().Cleanup(srv.Close)

		dc, err := discovery.NewDiscoveryClientForConfig(&rest.Config{Host: srv.URL})
		s.Require().NoError(err)
		s.True(clusterIsOpenShiftFromDiscovery(dc))
	})
}

func TestOpenShift(t *testing.T) {
	suite.Run(t, new(OpenShiftSuite))
}
