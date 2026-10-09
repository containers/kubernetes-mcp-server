package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	netobservToolset "github.com/containers/kubernetes-mcp-server/pkg/toolsets/netobserv"
	"github.com/stretchr/testify/suite"
	apidiscovery "k8s.io/api/apidiscovery/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type NetObservFilteringSuite struct {
	suite.Suite
	mockServer *test.MockServer
}

func TestNetObservFiltering(t *testing.T) {
	suite.Run(t, new(NetObservFilteringSuite))
}

func (s *NetObservFilteringSuite) SetupTest() {
	s.mockServer = test.NewMockServer()
}

func (s *NetObservFilteringSuite) TearDownTest() {
	s.mockServer.Close()
}

func (s *NetObservFilteringSuite) configuration(enabled bool, extra string) *config.Config {
	name := (&netobservToolset.Toolset{}).GetName()
	cfg, err := config.ReadToml(s.T().Context(), []byte(fmt.Sprintf(`
		toolsets = [%q]
		experimental_enable_target_compatibility_tool_filters = %t
		%s
	`, name, enabled, extra)), config.WithBaseDefault())
	s.Require().NoError(err)
	cfg.KubeConfig.SetForTest(s.mockServer.KubeconfigFile(s.T()))
	cfg.ReadOnly.SetForTest(false)
	return cfg
}

func (s *NetObservFilteringSuite) newClient(cfg *config.Config) (*Server, *test.McpClient) {
	provider, err := kubernetes.NewProvider(s.T().Context(), cfg)
	s.Require().NoError(err)
	s.T().Cleanup(provider.Close)
	server, err := NewServer(s.T().Context(), Configuration{Config: cfg}, provider)
	s.Require().NoError(err)
	s.T().Cleanup(server.Close)
	client := test.NewMcpClient(s.T(), server.ServeHTTP())
	s.T().Cleanup(client.Close)
	return server, client
}

func (s *NetObservFilteringSuite) assertTools(client *test.McpClient, available bool) {
	tools, err := client.ListTools()
	s.Require().NoError(err)
	names := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	name := (&netobservToolset.Toolset{}).GetName()
	for _, suffix := range []string{"list_flows", "get_flow_metrics", "export_flows"} {
		if available {
			s.Contains(names, name+"_"+suffix)
		} else {
			s.NotContains(names, name+"_"+suffix)
		}
	}
}

func (s *NetObservFilteringSuite) TestAvailability() {
	for _, tc := range []struct {
		name      string
		enabled   bool
		version   string
		kind      string
		extra     string
		available bool
	}{
		{name: "disabled without API", available: true},
		{name: "enabled without API", enabled: true},
		{name: "enabled with v1beta1 API", enabled: true, version: "v1beta1", kind: "FlowCollector", available: true},
		{name: "enabled with v1beta2 API", enabled: true, version: "v1beta2", kind: "FlowCollector", available: true},
		{name: "enabled with future API version", enabled: true, version: "v99", kind: "FlowCollector", available: true},
		{name: "group present without FlowCollector", enabled: true, version: "v1beta2", kind: "OtherKind"},
		{name: "explicit URL without API", enabled: true, extra: `[toolset_configs.netobserv]
url = "http://external-plugin.example:9001"`, available: true},
		{name: "whitespace URL without API", enabled: true, extra: `[toolset_configs.netobserv]
url = "   "`},
		{name: "service override without API", enabled: true, extra: `[toolset_configs.netobserv]
namespace = "custom"
service = "custom-plugin"`},
	} {
		s.Run(tc.name, func() {
			s.mockServer.ResetHandlers()
			var resources []metav1.APIResourceList
			if tc.version != "" {
				resources = append(resources, metav1.APIResourceList{
					GroupVersion: "flows.netobserv.io/" + tc.version,
					APIResources: []metav1.APIResource{{Name: "flowcollectors", Kind: tc.kind}},
				})
			}
			s.mockServer.Handle(test.NewDiscoveryClientHandler(resources...))
			_, client := s.newClient(s.configuration(tc.enabled, tc.extra))
			s.assertTools(client, tc.available)
		})
	}
}

func (s *NetObservFilteringSuite) TestDiscoveryErrorKeepsTools() {
	handler := test.NewDiscoveryClientHandler(metav1.APIResourceList{
		GroupVersion: "flows.netobserv.io/v1beta2",
		APIResources: []metav1.APIResource{{Name: "flowcollectors", Kind: "FlowCollector"}},
	})
	s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/apis/flows.netobserv.io/v1beta2" {
			http.Error(w, "discovery unavailable", http.StatusServiceUnavailable)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	_, client := s.newClient(s.configuration(true, ""))
	s.assertTools(client, true)
}

func (s *NetObservFilteringSuite) TestAggregatedDiscovery() {
	for _, tc := range []struct {
		name       string
		staleGroup string
		freshKind  string
		available  bool
	}{
		{name: "all relevant versions stale", staleGroup: "flows.netobserv.io", available: true},
		{name: "stale version and healthy version without kind", staleGroup: "flows.netobserv.io", freshKind: "OtherKind", available: true},
		{name: "healthy version confirms kind despite stale version", staleGroup: "flows.netobserv.io", freshKind: "FlowCollector", available: true},
		{name: "unrelated stale group does not enable NetObserv", staleGroup: "unrelated.example"},
		{name: "healthy group without FlowCollector", staleGroup: "unrelated.example", freshKind: "OtherKind"},
	} {
		s.Run(tc.name, func() {
			s.mockServer.ResetHandlers()
			groups := []apidiscovery.APIGroupDiscovery{{
				ObjectMeta: metav1.ObjectMeta{Name: tc.staleGroup},
				Versions:   []apidiscovery.APIVersionDiscovery{{Version: "v1beta1", Freshness: apidiscovery.DiscoveryFreshnessStale}},
			}}
			if tc.freshKind != "" {
				groups = append(groups, apidiscovery.APIGroupDiscovery{
					ObjectMeta: metav1.ObjectMeta{Name: "flows.netobserv.io"},
					Versions: []apidiscovery.APIVersionDiscovery{{
						Version: "v1beta2", Freshness: apidiscovery.DiscoveryFreshnessCurrent,
						Resources: []apidiscovery.APIResourceDiscovery{{
							Resource: "flowcollectors", Scope: apidiscovery.ScopeCluster,
							ResponseKind: &metav1.GroupVersionKind{Group: "flows.netobserv.io", Version: "v1beta2", Kind: tc.freshKind},
						}},
					}},
				})
			}
			s.mockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				list := apidiscovery.APIGroupDiscoveryList{
					TypeMeta: metav1.TypeMeta{APIVersion: "apidiscovery.k8s.io/v2", Kind: "APIGroupDiscoveryList"},
				}
				if r.URL.Path == "/apis" {
					list.Items = groups
				} else if r.URL.Path != "/api" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json;g=apidiscovery.k8s.io;v=v2;as=APIGroupDiscoveryList")
				if err := json.NewEncoder(w).Encode(list); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
			}))
			_, client := s.newClient(s.configuration(true, ""))
			s.assertTools(client, tc.available)
		})
	}
}

func (s *NetObservFilteringSuite) TestAnyTargetCanEnableTools() {
	s.mockServer.Handle(test.NewDiscoveryClientHandler())
	other := test.NewMockServer()
	s.T().Cleanup(other.Close)
	other.Handle(test.NewDiscoveryClientHandler(metav1.APIResourceList{
		GroupVersion: "flows.netobserv.io/v99",
		APIResources: []metav1.APIResource{{Name: "flowcollectors", Kind: "FlowCollector"}},
	}))
	kubeconfig := s.mockServer.Kubeconfig()
	kubeconfig.Clusters["netobserv"] = &clientcmdapi.Cluster{Server: other.Config().Host}
	kubeconfig.Contexts["netobserv"] = &clientcmdapi.Context{
		Cluster: "netobserv", AuthInfo: kubeconfig.Contexts[kubeconfig.CurrentContext].AuthInfo,
	}
	cfg := s.configuration(true, "")
	cfg.KubeConfig.SetForTest(test.KubeconfigFile(s.T(), kubeconfig))
	_, client := s.newClient(cfg)
	s.assertTools(client, true)
}

func (s *NetObservFilteringSuite) TestReloadExplicitURL() {
	s.mockServer.Handle(test.NewDiscoveryClientHandler())
	server, client := s.newClient(s.configuration(true, ""))
	s.assertTools(client, false)

	withURL := s.configuration(true, `[toolset_configs.netobserv]
url = "http://external-plugin.example:9001"`)
	s.Require().NoError(server.ReloadConfiguration(s.T().Context(), withURL))
	s.assertTools(client, true)

	s.Require().NoError(server.ReloadConfiguration(s.T().Context(), s.configuration(true, "")))
	s.assertTools(client, false)

	s.Require().NoError(server.ReloadConfiguration(s.T().Context(), s.configuration(false, "")))
	s.assertTools(client, true)
}
