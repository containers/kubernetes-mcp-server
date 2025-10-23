package mcp

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	configuration "github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/config"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/core"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/helm"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type ToolsetsSuite struct {
	suite.Suite
	originalToolsets []api.Toolset
	*test.MockServer
	*test.McpClient
	Cfg       *configuration.StaticConfig
	mcpServer *Server
}

func (s *ToolsetsSuite) SetupTest() {
	s.originalToolsets = toolsets.Toolsets()
	s.MockServer = test.NewMockServer()
	s.Cfg = configuration.Default()
	s.Cfg.KubeConfig = s.KubeconfigFile(s.T())
}

func (s *ToolsetsSuite) TearDownTest() {
	toolsets.Clear()
	for _, toolset := range s.originalToolsets {
		toolsets.Register(toolset)
	}
	s.MockServer.Close()
}

func (s *ToolsetsSuite) TearDownSubTest() {
	if s.McpClient != nil {
		s.McpClient.Close()
	}
	if s.mcpServer != nil {
		s.mcpServer.Close()
	}
}

func (s *ToolsetsSuite) TestNoToolsets() {
	s.Run("No toolsets registered", func() {
		toolsets.Clear()
		s.Cfg.Toolsets = []string{}
		s.InitMcpClient()
		tools, err := s.ListTools(s.T().Context(), mcp.ListToolsRequest{})
		s.Run("ListTools returns no tools", func() {
			s.NotNil(tools, "Expected tools from ListTools")
			s.NoError(err, "Expected no error from ListTools")
			s.Empty(tools.Tools, "Expected no tools from ListTools")
		})
	})
}

func (s *ToolsetsSuite) TestDefaultToolsetsTools() {
	if configuration.HasDefaultOverrides() {
		s.T().Skip("Skipping test because default configuration overrides are present (this is a downstream fork)")
	}
	s.Run("Default configuration toolsets", func() {
		s.InitMcpClient()
		tools, err := s.ListTools(s.T().Context(), mcp.ListToolsRequest{})
		s.Run("ListTools returns tools", func() {
			s.NotNil(tools, "Expected tools from ListTools")
			s.NoError(err, "Expected no error from ListTools")
		})
		s.Run("ListTools returns correct Tool metadata", func() {
			expectedMetadata := test.ReadFile("testdata", "toolsets-full-tools.json")
			metadata, err := json.MarshalIndent(tools.Tools, "", "  ")
			s.Require().NoErrorf(err, "failed to marshal tools metadata: %v", err)
			s.JSONEq(expectedMetadata, string(metadata), "tools metadata does not match expected")
		})
	})
}

func (s *ToolsetsSuite) TestDefaultToolsetsToolsInOpenShift() {
	if configuration.HasDefaultOverrides() {
		s.T().Skip("Skipping test because default configuration overrides are present (this is a downstream fork)")
	}
	s.Run("Default configuration toolsets in OpenShift", func() {
		s.Handle(&test.InOpenShiftHandler{})
		s.InitMcpClient()
		tools, err := s.ListTools(s.T().Context(), mcp.ListToolsRequest{})
		s.Run("ListTools returns tools", func() {
			s.NotNil(tools, "Expected tools from ListTools")
			s.NoError(err, "Expected no error from ListTools")
		})
		s.Run("ListTools returns correct Tool metadata", func() {
			expectedMetadata := test.ReadFile("testdata", "toolsets-full-tools-openshift.json")
			metadata, err := json.MarshalIndent(tools.Tools, "", "  ")
			s.Require().NoErrorf(err, "failed to marshal tools metadata: %v", err)
			s.JSONEq(expectedMetadata, string(metadata), "tools metadata does not match expected")
		})
	})
}

func (s *ToolsetsSuite) TestDefaultToolsetsToolsInMultiCluster() {
	if configuration.HasDefaultOverrides() {
		s.T().Skip("Skipping test because default configuration overrides are present (this is a downstream fork)")
	}
	s.Run("Default configuration toolsets in multi-cluster (with 11 clusters)", func() {
		kubeconfig := s.Kubeconfig()
		for i := 0; i < 10; i++ {
			// Add multiple fake contexts to force multi-cluster behavior
			kubeconfig.Contexts[strconv.Itoa(i)] = clientcmdapi.NewContext()
		}
		s.Cfg.KubeConfig = test.KubeconfigFile(s.T(), kubeconfig)
		s.InitMcpClient()
		tools, err := s.ListTools(s.T().Context(), mcp.ListToolsRequest{})
		s.Run("ListTools returns tools", func() {
			s.NotNil(tools, "Expected tools from ListTools")
			s.NoError(err, "Expected no error from ListTools")
		})
		s.Run("ListTools returns correct Tool metadata", func() {
			expectedMetadata := test.ReadFile("testdata", "toolsets-full-tools-multicluster.json")
			metadata, err := json.MarshalIndent(tools.Tools, "", "  ")
			s.Require().NoErrorf(err, "failed to marshal tools metadata: %v", err)
			s.JSONEq(expectedMetadata, string(metadata), "tools metadata does not match expected")
		})
	})
}

func (s *ToolsetsSuite) TestDefaultToolsetsToolsInMultiClusterEnum() {
	if configuration.HasDefaultOverrides() {
		s.T().Skip("Skipping test because default configuration overrides are present (this is a downstream fork)")
	}
	s.Run("Default configuration toolsets in multi-cluster (with 2 clusters)", func() {
		kubeconfig := s.Kubeconfig()
		// Add additional cluster to force multi-cluster behavior with enum parameter
		kubeconfig.Contexts["extra-cluster"] = clientcmdapi.NewContext()
		s.Cfg.KubeConfig = test.KubeconfigFile(s.T(), kubeconfig)
		s.InitMcpClient()
		tools, err := s.ListTools(s.T().Context(), mcp.ListToolsRequest{})
		s.Run("ListTools returns tools", func() {
			s.NotNil(tools, "Expected tools from ListTools")
			s.NoError(err, "Expected no error from ListTools")
		})
		s.Run("ListTools returns correct Tool metadata", func() {
			expectedMetadata := test.ReadFile("testdata", "toolsets-full-tools-multicluster-enum.json")
			metadata, err := json.MarshalIndent(tools.Tools, "", "  ")
			s.Require().NoErrorf(err, "failed to marshal tools metadata: %v", err)
			s.JSONEq(expectedMetadata, string(metadata), "tools metadata does not match expected")
		})
	})
}

func (s *ToolsetsSuite) TestGranularToolsetsTools() {
	testCases := []api.Toolset{
		&core.Toolset{},
		&config.Toolset{},
		&helm.Toolset{},
	}
	for _, testCase := range testCases {
		s.Run("Toolset "+testCase.GetName(), func() {
			toolsets.Clear()
			toolsets.Register(testCase)
			s.Cfg.Toolsets = []string{testCase.GetName()}
			s.InitMcpClient()
			tools, err := s.ListTools(s.T().Context(), mcp.ListToolsRequest{})
			s.Run("ListTools returns tools", func() {
				s.NotNil(tools, "Expected tools from ListTools")
				s.NoError(err, "Expected no error from ListTools")
			})
			s.Run("ListTools returns correct Tool metadata", func() {
				expectedMetadata := test.ReadFile("testdata", "toolsets-"+testCase.GetName()+"-tools.json")
				metadata, err := json.MarshalIndent(tools.Tools, "", "  ")
				s.Require().NoErrorf(err, "failed to marshal tools metadata: %v", err)
				s.JSONEq(expectedMetadata, string(metadata), "tools metadata does not match expected")
			})
		})
	}
}

func (s *ToolsetsSuite) TestInputSchemaEdgeCases() {
	//https://github.com/containers/kubernetes-mcp-server/issues/340
	s.Run("InputSchema for no-arg tool is object with empty properties", func() {
		s.InitMcpClient()
		tools, err := s.ListTools(s.T().Context(), mcp.ListToolsRequest{})
		s.Run("ListTools returns tools", func() {
			s.NotNil(tools, "Expected tools from ListTools")
			s.NoError(err, "Expected no error from ListTools")
		})
		var namespacesList *mcp.Tool
		for _, tool := range tools.Tools {
			if tool.Name == "namespaces_list" {
				namespacesList = &tool
				break
			}
		}
		s.Require().NotNil(namespacesList, "Expected namespaces_list from ListTools")
		s.NotNil(namespacesList.InputSchema.Properties, "Expected namespaces_list.InputSchema.Properties not to be nil")
		s.Empty(namespacesList.InputSchema.Properties, "Expected namespaces_list.InputSchema.Properties to be empty")
	})
}

func (s *ToolsetsSuite) InitMcpClient() {
	var err error
	s.mcpServer, err = NewServer(Configuration{StaticConfig: s.Cfg})
	s.Require().NoError(err, "Expected no error creating MCP server")
	s.McpClient = test.NewMcpClient(s.T(), s.mcpServer.ServeHTTP(nil))
}

func TestToolsets(t *testing.T) {
	suite.Run(t, new(ToolsetsSuite))
}
