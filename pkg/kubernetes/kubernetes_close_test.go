package kubernetes

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DerivedClientCleanupSuite struct {
	suite.Suite
	mu          sync.Mutex
	activeConns map[net.Conn]struct{}
	server      *httptest.Server
	manager     *Manager
}

func (s *DerivedClientCleanupSuite) SetupTest() {
	s.activeConns = make(map[net.Conn]struct{})

	handler := test.NewDiscoveryClientHandler()
	s.server = httptest.NewUnstartedServer(handler)
	s.server.Config.ConnState = func(conn net.Conn, state http.ConnState) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if state == http.StateClosed {
			delete(s.activeConns, conn)
		} else {
			s.activeConns[conn] = struct{}{}
		}
	}
	s.server.Start()

	kubeconfig := test.KubeConfigFake()
	kubeconfig.Clusters["fake"].Server = s.server.URL
	kubeconfigFile := test.KubeconfigFile(s.T(), kubeconfig)

	cfg := test.Must(config.ReadToml([]byte(`kubeconfig = "` + strings.ReplaceAll(kubeconfigFile, `\`, `\\`) + `"`)))

	var err error
	s.manager, err = NewKubeconfigManager(cfg, "")
	s.Require().NoError(err)
}

func (s *DerivedClientCleanupSuite) TearDownTest() {
	if s.server != nil {
		s.server.Close()
	}
}

func (s *DerivedClientCleanupSuite) activeConnCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.activeConns)
}

func (s *DerivedClientCleanupSuite) TestClosesIdleConnectionsWhenContextIsCancelled() {
	// https://github.com/containers/kubernetes-mcp-server/issues/830
	// https://github.com/containers/kubernetes-mcp-server/pull/850
	s.T().Skip("Pending fix: derived client must close idle connections when its context is cancelled")
	baseConns := s.activeConnCount()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = context.WithValue(ctx, OAuthAuthorizationHeader, "Bearer test-token")

	derived, err := s.manager.Derived(ctx)
	s.Require().NoError(err)
	s.NotEqual(derived, s.manager.kubernetes, "expected a new derived client, not the base client")

	// Make requests through the derived client to establish TCP connections.
	_, err = derived.DiscoveryClient().ServerGroups()
	s.Require().NoError(err, "discovery call should succeed against mock server")
	_, _ = derived.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})

	// Wait for connections to be established
	s.Require().Eventually(func() bool {
		return s.activeConnCount() > baseConns
	}, 2*time.Second, 10*time.Millisecond,
		"expected connections from derived client API calls",
	)

	// Cancel the context - with a correct fix, this should trigger cleanup
	cancel()

	s.Eventually(func() bool {
		return s.activeConnCount() <= baseConns
	}, 2*time.Second, 100*time.Millisecond,
		"expected derived client connections to be closed after context cancellation",
	)
}

func (s *DerivedClientCleanupSuite) TestMultipleDerivedClientsCleanedUpAfterContextCancellation() {
	// https://github.com/containers/kubernetes-mcp-server/issues/830
	// https://github.com/containers/kubernetes-mcp-server/pull/850
	s.T().Skip("Pending fix: derived client must close idle connections when its context is cancelled")
	// client-go caches base http.Transport instances by TLS config, so all
	// derived clients connecting to the same server share the same transport
	// and TCP connection pool. This test verifies that when ALL derived client
	// contexts are cancelled, the shared pool's idle connections are cleaned up.
	iterations := 5
	cancelFuncs := make([]context.CancelFunc, 0, iterations)

	for i := 0; i < iterations; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		ctx = context.WithValue(ctx, OAuthAuthorizationHeader, "Bearer test-token")

		derived, err := s.manager.Derived(ctx)
		s.Require().NoError(err)
		s.NotEqual(derived, s.manager.kubernetes)

		_, _ = derived.DiscoveryClient().ServerGroups()
		cancelFuncs = append(cancelFuncs, cancel)
	}

	s.Require().Eventually(func() bool {
		return s.activeConnCount() > 1
	}, 2*time.Second, 10*time.Millisecond,
		"expected multiple connections to exist before cleanup",
	)

	for _, cancel := range cancelFuncs {
		cancel()
	}

	s.Eventually(func() bool {
		return s.activeConnCount() == 0
	}, 2*time.Second, 100*time.Millisecond,
		"expected all shared transport connections to be closed after all "+
			"derived client contexts are cancelled",
	)
}

func TestDerivedClientCleanup(t *testing.T) {
	suite.Run(t, new(DerivedClientCleanupSuite))
}
