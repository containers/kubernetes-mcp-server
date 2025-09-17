package mcp

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/rest"
	v1 "k8s.io/client-go/tools/clientcmd/api/v1"
	"sigs.k8s.io/yaml"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

type ConfigurationSuite struct {
	suite.Suite
	*test.McpClient
	mcpServer *Server
	Cfg       *config.StaticConfig
}

func (s *ConfigurationSuite) SetupTest() {
	s.Cfg = config.Default()
}

func (s *ConfigurationSuite) TearDownTest() {
	if s.McpClient != nil {
		s.McpClient.Close()
	}
	if s.mcpServer != nil {
		s.mcpServer.Close()
	}
}

func (s *ConfigurationSuite) InitMcpClient() {
	var err error
	s.mcpServer, err = NewServer(Configuration{StaticConfig: s.Cfg})
	s.Require().NoError(err, "Expected no error creating MCP server")
	s.McpClient = test.NewMcpClient(s.T(), s.mcpServer.ServeHTTP(nil))
}

func (s *ConfigurationSuite) TestConfigurationView() {
	// Out of cluster requires kubeconfig
	mockServer := test.NewMockServer()
	s.T().Cleanup(mockServer.Close)
	s.Cfg.KubeConfig = mockServer.KubeconfigFile(s.T())
	s.InitMcpClient()
	s.Run("configuration_view", func() {
		toolResult, err := s.CallTool("configuration_view", map[string]interface{}{})
		s.Run("returns configuration", func() {
			s.Nilf(err, "call tool failed %v", err)
		})
		s.Require().NotNil(toolResult, "Expected tool result from call")
		var decoded *v1.Config
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decoded)
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
	s.Run("configuration_view(minified=false)", func() {
		toolResult, err := s.CallTool("configuration_view", map[string]interface{}{
			"minified": false,
		})
		s.Run("returns configuration", func() {
			s.Nilf(err, "call tool failed %v", err)
		})
		var decoded *v1.Config
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decoded)
		s.Run("has yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
		})
		s.Run("returns additional context info", func() {
			s.Lenf(decoded.Contexts, 2, "invalid context count, expected 2, got %v", len(decoded.Contexts))
			s.Equalf("additional-context", decoded.Contexts[0].Name, "additional-context not found: %v", decoded.Contexts)
			s.Equalf("additional-cluster", decoded.Contexts[0].Context.Cluster, "additional-cluster not found: %v", decoded.Contexts)
			s.Equalf("additional-auth", decoded.Contexts[0].Context.AuthInfo, "additional-auth not found: %v", decoded.Contexts)
			s.Equalf("fake-context", decoded.Contexts[1].Name, "fake-context not found: %v", decoded.Contexts)
		})
		s.Run("returns cluster info", func() {
			s.Lenf(decoded.Clusters, 2, "invalid cluster count, expected 2, got %v", len(decoded.Clusters))
			s.Equalf("additional-cluster", decoded.Clusters[0].Name, "additional-cluster not found: %v", decoded.Clusters)
		})
		s.Run("configuration_view with minified=false returns auth info", func() {
			s.Lenf(decoded.AuthInfos, 2, "invalid auth info count, expected 2, got %v", len(decoded.AuthInfos))
			s.Equalf("additional-auth", decoded.AuthInfos[0].Name, "additional-auth not found: %v", decoded.AuthInfos)
		})
	})
}

func (s *ConfigurationSuite) TestConfigurationViewInCluster() {
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
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decoded)
		s.Run("has yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
		})
		s.Run("returns current-context", func() {
			s.Equalf("context", decoded.CurrentContext, "context not found: %v", decoded.CurrentContext)
		})
		s.Run("returns context info", func() {
			s.Lenf(decoded.Contexts, 1, "invalid context count, expected 1, got %v", len(decoded.Contexts))
			s.Equalf("context", decoded.Contexts[0].Name, "context not found: %v", decoded.Contexts)
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
