package kcp_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	_ "github.com/containers/kubernetes-mcp-server/pkg/kcp"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

type ImpersonationProviderSuite struct{ suite.Suite }

func TestImpersonationProvider(t *testing.T) { suite.Run(t, new(ImpersonationProviderSuite)) }

func (s *ImpersonationProviderSuite) TestStartupAndWatchersDoNotNeedCallerIdentity() {
	discovery := test.NewDiscoveryClientHandler(metav1.APIResourceList{
		GroupVersion: "tenancy.kcp.io/v1alpha1",
		APIResources: []metav1.APIResource{{Name: "workspaces", Kind: "Workspace", Verbs: metav1.Verbs{"list"}}},
	})
	var workspaceReads atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("Bearer backend-token", r.Header.Get("Authorization"))
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/clusters/root")
		switch r.URL.Path {
		case "/apis/tenancy.kcp.io/v1alpha1/workspaces":
			workspaceReads.Add(1)
			s.Empty(r.Header.Get("Impersonate-User"), "workspace inventory is a background operation")
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"apiVersion":"tenancy.kcp.io/v1alpha1","kind":"WorkspaceList","items":[]}`)
		case "/version":
			s.Equal("alice", r.Header.Get("Impersonate-User"), "caller requests must still impersonate")
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"major":"1","minor":"37","gitVersion":"v1.37.0"}`)
		default:
			discovery.ServeHTTP(w, r)
		}
	}))
	defer server.Close()
	raw := test.KubeConfigFake()
	raw.Clusters["fake"].Server = server.URL + "/clusters/root"
	raw.Clusters["fake"].InsecureSkipTLSVerify = true
	raw.AuthInfos["fake"].Token = "backend-token"
	cfg := config.BaseDefault()
	cfg.KubeConfig.SetForTest(test.KubeconfigFile(s.T(), raw))
	cfg.ClusterProviderStrategy.SetForTest(config.ClusterProviderKcp)
	cfg.ClusterAuthMode.SetForTest(config.ClusterAuthImpersonation)
	ctx, cancel := context.WithCancel(s.T().Context())
	defer cancel()
	provider, err := kubernetes.NewProvider(ctx, cfg)
	s.Require().NoError(err, "provider bootstrap has no MCP caller")
	defer provider.Close()
	provider.WatchTargets(ctx, kubernetes.McpReloaderFromCallback(func() error { return nil }))
	s.GreaterOrEqual(workspaceReads.Load(), int32(2), "startup and the watcher must both read workspace inventory")

	_, err = provider.GetDerivedKubernetes(ctx, "root")
	s.ErrorContains(err, "identity required")
	callerCtx := kubernetes.WithImpersonationIdentity(ctx, kubernetes.ImpersonationIdentity{UserName: "alice"})
	client, err := provider.GetDerivedKubernetes(callerCtx, "root")
	s.Require().NoError(err)
	version, err := client.DiscoveryClient().ServerVersion()
	s.Require().NoError(err)
	s.Equal("v1.37.0", version.GitVersion)
}
