package mcp

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	v1 "k8s.io/client-go/tools/clientcmd/api/v1"
	"sigs.k8s.io/yaml"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

type ConfigurationSuite struct {
	BaseMcpSuite
}

func (s *ConfigurationSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	// Use mock server for predictable kubeconfig content
	mockServer := test.NewMockServer()
	s.T().Cleanup(mockServer.Close)
	kubeconfig := mockServer.Kubeconfig()
	for i := 0; i < 11; i++ {
		// Add multiple fake contexts to force the context_list tool to appear
		// and test minification in configuration_view tool. cluster-10 is included
		// so TestContextsList can assert the lexicographic ordering contract
		// (cluster-10 sorts before cluster-2).
		name := fmt.Sprintf("cluster-%d", i)
		kubeconfig.Contexts[name] = clientcmdapi.NewContext()
		kubeconfig.Clusters[name+"-cluster"] = clientcmdapi.NewCluster()
		kubeconfig.AuthInfos[name+"-auth"] = clientcmdapi.NewAuthInfo()
		kubeconfig.Contexts[name].Cluster = name + "-cluster"
		kubeconfig.Contexts[name].AuthInfo = name + "-auth"
	}
	s.Cfg.KubeConfig.SetForTest(test.KubeconfigFile(s.T(), kubeconfig))
}

func (s *ConfigurationSuite) TestContextsList() {
	s.InitMcpClient()
	s.Run("context_list", func() {
		toolResult, err := s.CallTool("context_list", map[string]interface{}{})
		s.Run("returns contexts", func() {
			s.Nilf(err, "call tool failed %v", err)
		})
		s.Require().NotNil(toolResult, "Expected tool result from call")
		s.Lenf(toolResult.Content, 1, "invalid tool result content length %v", len(toolResult.Content))
		text := toolResult.Content[0].(*mcp.TextContent).Text
		s.Run("names the default and every context with its server", func() {
			s.Regexp(`(?m)^default: fake-context$`, text)
			s.Equal(12, strings.Count(text, "  name: "))
			s.Regexp(`(?m)^  name: fake-context\n  server: http://127\.0\.0\.1:\d+$`, text)
		})
		s.Run("returns structured targets", func() {
			structured, ok := toolResult.StructuredContent.(map[string]interface{})
			s.Require().True(ok, "expected structured result")
			s.Equal("fake-context", structured["default"])
			items, ok := structured["targets"].([]interface{})
			s.Require().True(ok, "expected targets array")
			s.Len(items, 12)
			first, ok := items[0].(map[string]interface{})
			s.Require().True(ok, "expected target entry object")
			s.Equal("cluster-0", first["name"])
			s.Equal(false, first["default"])
			s.Nil(first["server"], "cluster-0 has no server and the key is omitted")
			// Lock lexicographic ordering: cluster-10 sorts between cluster-1
			// and cluster-2, so items[2] must be "cluster-10".
			middle, ok := items[2].(map[string]interface{})
			s.Require().True(ok, "expected target entry object")
			s.Equal("cluster-10", middle["name"], "expected lexicographic ordering: items[2] should be cluster-10")
			last, ok := items[11].(map[string]interface{})
			s.Require().True(ok, "expected target entry object")
			s.Equal("fake-context", last["name"])
			s.Equal(true, last["default"])
			s.Regexp(`^https?://(127\.0\.0\.1|localhost):\d+$`, last["server"], "expected the server URL of fake-context")
		})
	})
}

func (s *ConfigurationSuite) TestConfigurationViewHiddenOverHTTPByDefault() {
	s.Cfg.Port.SetForTest("8080")
	s.InitMcpClient()

	tools, err := s.ListTools()
	s.Require().NoError(err)
	s.False(slices.ContainsFunc(tools.Tools, func(tool *mcp.Tool) bool {
		return tool.Name == "configuration_view"
	}))
}

func (s *ConfigurationSuite) TestConfigurationViewExplicitlyEnabledOverHTTP() {
	s.Cfg.Port.SetForTest("8080")
	s.Cfg.EnabledTools.SetForTest([]string{"configuration_view"})
	s.InitMcpClient()

	tools, err := s.ListTools()
	s.Require().NoError(err)
	s.True(slices.ContainsFunc(tools.Tools, func(tool *mcp.Tool) bool {
		return tool.Name == "configuration_view"
	}))
}

func (s *ConfigurationSuite) TestConfigurationView() {
	s.InitMcpClient()
	s.Run("configuration_view", func() {
		toolResult, err := s.CallTool("configuration_view", map[string]interface{}{})
		s.Run("returns configuration", func() {
			s.Nilf(err, "call tool failed %v", err)
		})
		s.Require().NotNil(toolResult, "Expected tool result from call")
		var decoded *v1.Config
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(*mcp.TextContent).Text), &decoded)
		s.Run("has yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
		})
		s.Run("returns current-context", func() {
			s.Equalf("fake-context", decoded.CurrentContext, "fake-context not found: %v", decoded.CurrentContext)
		})
		s.Run("returns context info", func() {
			s.Lenf(decoded.Contexts, 1, "invalid context count, expected 1, got %v", len(decoded.Contexts))
			s.Equalf("fake-context", decoded.Contexts[0].Name, "fake-context not found: %v", decoded.Contexts)
			s.Equalf("fake", decoded.Contexts[0].Context.Cluster, "fake-cluster not found: %v", decoded.Contexts)
			s.Equalf("fake", decoded.Contexts[0].Context.AuthInfo, "fake-auth not found: %v", decoded.Contexts)
		})
		s.Run("returns cluster info", func() {
			s.Lenf(decoded.Clusters, 1, "invalid cluster count, expected 1, got %v", len(decoded.Clusters))
			s.Equalf("fake", decoded.Clusters[0].Name, "fake-cluster not found: %v", decoded.Clusters)
			s.Regexpf(`^https?://(127\.0\.0\.1|localhost):\d{1,5}$`, decoded.Clusters[0].Cluster.Server, "fake-server not found: %v", decoded.Clusters)
		})
		s.Run("returns auth info", func() {
			s.Lenf(decoded.AuthInfos, 1, "invalid auth info count, expected 1, got %v", len(decoded.AuthInfos))
			s.Equalf("fake", decoded.AuthInfos[0].Name, "fake-auth not found: %v", decoded.AuthInfos)
		})
	})
	s.Run("configuration_view with minified as string returns error without panicking", func() {
		toolResult, err := s.CallTool("configuration_view", map[string]interface{}{
			"minified": "false",
		})
		s.Nilf(err, "call tool should not return error object")
		s.Truef(toolResult.IsError, "call tool should fail with type mismatch")
		s.Equalf("failed to get configuration: minified parameter must be a boolean", toolResult.Content[0].(*mcp.TextContent).Text,
			"invalid error message, got %v", toolResult.Content[0].(*mcp.TextContent).Text)
	})
	s.Run("configuration_view(minified=false)", func() {
		toolResult, err := s.CallTool("configuration_view", map[string]interface{}{
			"minified": false,
		})
		s.Run("returns configuration", func() {
			s.Nilf(err, "call tool failed %v", err)
		})
		var decoded *v1.Config
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(*mcp.TextContent).Text), &decoded)
		s.Run("has yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
		})
		s.Run("returns additional context info", func() {
			s.Lenf(decoded.Contexts, 12, "invalid context count, expected 12, got %v", len(decoded.Contexts))
			s.Equalf("cluster-0", decoded.Contexts[0].Name, "cluster-0 not found: %v", decoded.Contexts)
			s.Equalf("cluster-0-cluster", decoded.Contexts[0].Context.Cluster, "cluster-0-cluster not found: %v", decoded.Contexts)
			s.Equalf("cluster-0-auth", decoded.Contexts[0].Context.AuthInfo, "cluster-0-auth not found: %v", decoded.Contexts)
			s.Equalf("fake", decoded.Contexts[11].Context.Cluster, "fake not found: %v", decoded.Contexts)
			s.Equalf("fake", decoded.Contexts[11].Context.AuthInfo, "fake not found: %v", decoded.Contexts)
			s.Equalf("fake-context", decoded.Contexts[11].Name, "fake-context not found: %v", decoded.Contexts)
		})
		s.Run("returns cluster info", func() {
			s.Lenf(decoded.Clusters, 12, "invalid cluster count, expected 12, got %v", len(decoded.Clusters))
			s.Equalf("cluster-0-cluster", decoded.Clusters[0].Name, "cluster-0-cluster not found: %v", decoded.Clusters)
			s.Equalf("fake", decoded.Clusters[11].Name, "fake not found: %v", decoded.Clusters)
		})
		s.Run("configuration_view with minified=false returns auth info", func() {
			s.Lenf(decoded.AuthInfos, 12, "invalid auth info count, expected 12, got %v", len(decoded.AuthInfos))
			s.Equalf("cluster-0-auth", decoded.AuthInfos[0].Name, "cluster-0-auth not found: %v", decoded.AuthInfos)
			s.Equalf("fake", decoded.AuthInfos[11].Name, "fake not found: %v", decoded.AuthInfos)
		})
	})
}

func (s *ConfigurationSuite) TestConfigurationViewInCluster() {
	s.Cfg.KubeConfig.SetForTest("") // Force in-cluster
	kubernetes.InClusterConfig = func() (*rest.Config, error) {
		return &rest.Config{
			Host:        "https://kubernetes.default.svc",
			BearerToken: "fake-token",
		}, nil
	}
	s.T().Cleanup(func() { kubernetes.InClusterConfig = rest.InClusterConfig })
	s.InitMcpClient()
	s.Run("configuration_view", func() {
		toolResult, err := s.CallTool("configuration_view", map[string]interface{}{})
		s.Run("returns configuration", func() {
			s.Nilf(err, "call tool failed %v", err)
		})
		s.Require().NotNil(toolResult, "Expected tool result from call")
		var decoded *v1.Config
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(*mcp.TextContent).Text), &decoded)
		s.Run("has yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
		})
		s.Run("returns current-context", func() {
			s.Equalf("in-cluster", decoded.CurrentContext, "context not found: %v", decoded.CurrentContext)
		})
		s.Run("returns context info", func() {
			s.Lenf(decoded.Contexts, 1, "invalid context count, expected 1, got %v", len(decoded.Contexts))
			s.Equalf("in-cluster", decoded.Contexts[0].Name, "context not found: %v", decoded.Contexts)
			s.Equalf("cluster", decoded.Contexts[0].Context.Cluster, "cluster not found: %v", decoded.Contexts)
			s.Equalf("user", decoded.Contexts[0].Context.AuthInfo, "user not found: %v", decoded.Contexts)
		})
		s.Run("returns cluster info", func() {
			s.Lenf(decoded.Clusters, 1, "invalid cluster count, expected 1, got %v", len(decoded.Clusters))
			s.Equalf("cluster", decoded.Clusters[0].Name, "cluster not found: %v", decoded.Clusters)
			s.Equalf("https://kubernetes.default.svc", decoded.Clusters[0].Cluster.Server, "server not found: %v", decoded.Clusters)
		})
		s.Run("returns auth info", func() {
			s.Lenf(decoded.AuthInfos, 1, "invalid auth info count, expected 1, got %v", len(decoded.AuthInfos))
			s.Equalf("user", decoded.AuthInfos[0].Name, "user not found: %v", decoded.AuthInfos)
		})
	})
}

func TestConfiguration(t *testing.T) {
	suite.Run(t, new(ConfigurationSuite))
}
