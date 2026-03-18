package mcp

import (
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/yaml"
)

const proxyPathPrefix = "/gateway/k8s/clusters/test"

type ProxiedKubernetesSuite struct {
	BaseMcpSuite
	proxy *httptest.Server
}

func (s *ProxiedKubernetesSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()

	// Create a reverse proxy that exposes the envtest API server behind a path prefix.
	// This simulates a gateway/proxy that serves the Kubernetes API under a sub-path.
	targetURL, err := url.Parse(envTestRestConfig.Host)
	s.Require().NoError(err, "Expected to parse envtest host URL")

	transport, err := rest.TransportFor(envTestRestConfig)
	s.Require().NoError(err, "Expected to create transport for envtest")

	s.proxy = httptest.NewServer(&httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(targetURL)
			r.Out.URL.Path = strings.TrimPrefix(r.In.URL.Path, proxyPathPrefix)
			if r.Out.URL.Path == "" {
				r.Out.URL.Path = "/"
			}
		},
		Transport: transport,
	})

	// Overwrite the kubeconfig with one that points to the proxy with the path prefix
	kubeConfig := clientcmdapi.NewConfig()
	kubeConfig.Clusters["test"] = &clientcmdapi.Cluster{
		Server: s.proxy.URL + proxyPathPrefix,
	}
	kubeConfig.AuthInfos["test-user"] = &clientcmdapi.AuthInfo{}
	kubeConfig.Contexts["test"] = &clientcmdapi.Context{
		Cluster:  "test",
		AuthInfo: "test-user",
	}
	kubeConfig.CurrentContext = "test"

	data, err := clientcmd.Write(*kubeConfig)
	s.Require().NoError(err, "Expected to serialize kubeconfig")
	s.Require().NoError(os.WriteFile(s.Cfg.KubeConfig, data, 0600), "Expected to write kubeconfig file")
}

func (s *ProxiedKubernetesSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	if s.proxy != nil {
		s.proxy.Close()
	}
}

func (s *ProxiedKubernetesSuite) TestPodsListThroughProxy() {
	s.InitMcpClient()
	s.Run("pods_list returns pods through path-prefixed proxy", func() {
		toolResult, err := s.CallTool("pods_list", map[string]interface{}{})
		s.Nilf(err, "call tool failed %v", err)
		s.Falsef(toolResult.IsError, "call tool failed: %s", toolResult.Content[0].(*mcp.TextContent).Text)
		var decoded []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(*mcp.TextContent).Text), &decoded)
		s.Nilf(err, "invalid tool result content %v", err)
		s.GreaterOrEqual(len(decoded), 3, "expected at least 3 pods")
	})
}

func (s *ProxiedKubernetesSuite) TestPodsListDeniedThroughProxy() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		denied_resources = [ { version = "v1", kind = "Pod" } ]
	`), s.Cfg), "Expected to parse denied resources config")
	s.InitMcpClient()
	s.Run("pods_list is denied through proxy", func() {
		toolResult, err := s.CallTool("pods_list", map[string]interface{}{})
		s.Nilf(err, "call tool should not return error object")
		s.True(toolResult.IsError, "call tool should fail")
		msg := toolResult.Content[0].(*mcp.TextContent).Text
		s.Contains(msg, "resource not allowed:")
	})
}

func TestProxiedKubernetes(t *testing.T) {
	suite.Run(t, new(ProxiedKubernetesSuite))
}
