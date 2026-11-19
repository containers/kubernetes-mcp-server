package kubernetes

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// resettable is satisfied by both singleClusterProvider and kubeConfigClusterProvider.
type resettable interface {
	reset(ctx context.Context) error
}

// ProviderCloseTestSuite verifies that provider reset() calls close HTTP
// transport resources (TCP sockets, TLS sessions, connection pools) held by
// old Manager instances before replacing them.
//
// https://github.com/containers/kubernetes-mcp-server/pull/977
type ProviderCloseTestSuite struct {
	suite.Suite
	mu          sync.Mutex
	activeConns map[net.Conn]struct{}
	server      *httptest.Server
	cfg         *config.Config
	providers   []Provider
}

func (s *ProviderCloseTestSuite) SetupTest() {
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
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("context-%d", i)
		kubeconfig.Contexts[name] = clientcmdapi.NewContext()
		kubeconfig.Contexts[name].Cluster = "fake"
		kubeconfig.Contexts[name].AuthInfo = "fake"
	}
	cfg := func() *config.Config {
		c := config.New()
		c.KubeConfig.SetForTest(test.KubeconfigFile(s.T(), kubeconfig))
		return c
	}()
	s.cfg = cfg

	singleProvider, err := newSingleClusterProvider(config.ClusterProviderDisabled)(s.T().Context(), cfg)
	s.Require().NoError(err)
	kubeconfigProvider, err := newKubeConfigClusterProvider(s.T().Context(), cfg)
	s.Require().NoError(err)

	s.providers = []Provider{singleProvider, kubeconfigProvider}
}

func (s *ProviderCloseTestSuite) TearDownTest() {
	for _, provider := range s.providers {
		provider.Close()
	}
	if s.server != nil {
		s.server.Close()
	}
}

// snapshotConns returns a copy of the currently active connections.
func (s *ProviderCloseTestSuite) snapshotConns() map[net.Conn]struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := make(map[net.Conn]struct{}, len(s.activeConns))
	for conn := range s.activeConns {
		snapshot[conn] = struct{}{}
	}
	return snapshot
}

// connsSince returns connections that are currently active but were NOT in the before snapshot.
func (s *ProviderCloseTestSuite) connsSince(before map[net.Conn]struct{}) map[net.Conn]struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	diff := make(map[net.Conn]struct{})
	for conn := range s.activeConns {
		if _, existed := before[conn]; !existed {
			diff[conn] = struct{}{}
		}
	}
	return diff
}

// allClosed returns true when every connection in the snapshot has been closed.
func (s *ProviderCloseTestSuite) allClosed(snapshot map[net.Conn]struct{}) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for conn := range snapshot {
		if _, still := s.activeConns[conn]; still {
			return false
		}
	}
	return true
}

func (s *ProviderCloseTestSuite) TestClosesOldConnectionsOnReset() {
	for _, provider := range s.providers {
		s.Run("closes old manager connections for "+reflect.TypeOf(provider).String(), func() {
			beforeConns := s.snapshotConns()

			k, err := provider.GetDerivedKubernetes(s.T().Context(), provider.GetDefaultTarget())
			s.Require().NoError(err)
			_, err = k.DiscoveryClient().ServerGroups()
			s.Require().NoError(err)

			s.Require().Eventually(func() bool {
				return len(s.connsSince(beforeConns)) > 0
			}, 2*time.Second, 10*time.Millisecond,
				"expected connections from provider")

			providerConns := s.connsSince(beforeConns)

			err = provider.(resettable).reset(s.T().Context())
			s.Require().NoError(err)

			s.Eventually(func() bool {
				return s.allClosed(providerConns)
			}, 2*time.Second, 100*time.Millisecond,
				"expected old manager connections to be closed after provider reset")
		})
	}
}

func (s *ProviderCloseTestSuite) TestClosesLazyContextConnectionsOnReset() {
	for _, provider := range s.providers {
		if !provider.IsMultiTarget() {
			continue
		}
		s.Run("closes lazily initialized context connections for "+reflect.TypeOf(provider).String(), func() {
			beforeConns := s.snapshotConns()

			targets, err := provider.GetTargets(s.T().Context())
			s.Require().NoError(err)
			var lazyTarget string
			for _, t := range targets {
				if t != provider.GetDefaultTarget() {
					lazyTarget = t
					break
				}
			}
			s.Require().NotEmpty(lazyTarget, "expected a non-default target for lazy initialization")

			for _, target := range []string{provider.GetDefaultTarget(), lazyTarget} {
				k, err := provider.GetDerivedKubernetes(s.T().Context(), target)
				s.Require().NoError(err)
				_, err = k.DiscoveryClient().ServerGroups()
				s.Require().NoError(err)
			}

			s.Require().Eventually(func() bool {
				return len(s.connsSince(beforeConns)) > 0
			}, 2*time.Second, 10*time.Millisecond,
				"expected connections from provider")

			providerConns := s.connsSince(beforeConns)

			err = provider.(resettable).reset(s.T().Context())
			s.Require().NoError(err)

			s.Eventually(func() bool {
				return s.allClosed(providerConns)
			}, 2*time.Second, 100*time.Millisecond,
				"expected all old manager connections (including lazily initialized) to be closed after reset")
		})
	}
}

// TestCloseRacesReset exercises Close concurrently with reset(), which
// installs a replacement manager while holding the provider lock. It guards
// the shutdown path taken in this change: Close reads the manager(s) under
// p.mu without deadlocking against reset's p.mu -> WatcherGroup lock order,
// stays idempotent when several closers run at once, and the whole sequence
// is race-clean under -race.
func (s *ProviderCloseTestSuite) TestCloseRacesReset() {
	factories := map[string]ProviderFactory{
		"single":     newSingleClusterProvider(config.ClusterProviderDisabled),
		"kubeconfig": newKubeConfigClusterProvider,
	}
	for name, factory := range factories {
		s.Run("closes safely while a reset is in flight for "+name, func() {
			for i := 0; i < 20; i++ {
				provider, err := factory(s.T().Context(), s.cfg)
				s.Require().NoError(err)

				var wg sync.WaitGroup
				wg.Add(1)
				go func() {
					defer wg.Done()
					// A reset that loses the race returns errProviderClosed.
					_ = provider.(resettable).reset(s.T().Context())
				}()
				// Several closers run the shutdown path concurrently with
				// the reset that is installing a replacement manager.
				for j := 0; j < 8; j++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						provider.Close()
					}()
				}
				wg.Wait()
				// Close stays idempotent after the race.
				provider.Close()
			}
		})
	}
}

func TestProviderClose(t *testing.T) {
	suite.Run(t, new(ProviderCloseTestSuite))
}
