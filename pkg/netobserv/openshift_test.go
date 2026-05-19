package netobserv

import (
	"net/http/httptest"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

func TestClusterIsOpenShift(t *testing.T) {
	t.Run("returns false without client", func(t *testing.T) {
		t.Parallel()
		if clusterIsOpenShift(nil) {
			t.Fatal("expected false")
		}
	})

	t.Run("returns false without discovery client", func(t *testing.T) {
		t.Parallel()
		if isOpenShiftDiscovery(nil) {
			t.Fatal("expected false")
		}
	})
}

func TestIsOpenShiftDiscovery(t *testing.T) {
	t.Run("returns true when project.openshift.io is registered", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(test.NewInOpenShiftHandler())
		t.Cleanup(srv.Close)

		dc, err := discovery.NewDiscoveryClientForConfig(&rest.Config{Host: srv.URL})
		if err != nil {
			t.Fatalf("discovery client: %v", err)
		}
		if !isOpenShiftDiscovery(dc) {
			t.Fatal("expected OpenShift cluster")
		}
	})

	t.Run("returns false on plain Kubernetes discovery", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(test.NewDiscoveryClientHandler())
		t.Cleanup(srv.Close)

		dc, err := discovery.NewDiscoveryClientForConfig(&rest.Config{Host: srv.URL})
		if err != nil {
			t.Fatalf("discovery client: %v", err)
		}
		if isOpenShiftDiscovery(dc) {
			t.Fatal("expected plain Kubernetes cluster")
		}
	})
}
