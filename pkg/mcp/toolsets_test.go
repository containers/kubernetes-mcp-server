package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	configuration "github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/code"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/config"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/core"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/helm"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kcp"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const updateJsonEnvVar = "UPDATE_TOOLSETS_JSON"

type ToolsetsSuite struct {
	suite.Suite
	originalToolsets []api.Toolset
	*test.MockServer
	*test.McpClient
	Cfg        *configuration.StaticConfig
	mcpServer  *Server
	updateJson bool
}

func (s *ToolsetsSuite) SetupTest() {
	s.originalToolsets = toolsets.Toolsets()
	s.MockServer = test.NewMockServer()
	s.Cfg = configuration.Default()
	s.Cfg.KubeConfig = s.KubeconfigFile(s.T())
	s.updateJson = os.Getenv(updateJsonEnvVar) != ""
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
			s.assertJsonSnapshot("toolsets-full-tools.json", tools.Tools)
		})
	})
}

func (s *ToolsetsSuite) TestDefaultToolsetsToolsInOpenShift() {
	if configuration.HasDefaultOverrides() {
		s.T().Skip("Skipping test because default configuration overrides are present (this is a downstream fork)")
	}
	s.Run("Default configuration toolsets in OpenShift", func() {
		s.Handle(test.NewInOpenShiftHandler())
		s.InitMcpClient()
		tools, err := s.ListTools(s.T().Context(), mcp.ListToolsRequest{})
		s.Run("ListTools returns tools", func() {
			s.NotNil(tools, "Expected tools from ListTools")
			s.NoError(err, "Expected no error from ListTools")
		})
		s.Run("ListTools returns correct Tool metadata", func() {
			s.assertJsonSnapshot("toolsets-full-tools-openshift.json", tools.Tools)
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
			s.assertJsonSnapshot("toolsets-full-tools-multicluster.json", tools.Tools)
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
			s.assertJsonSnapshot("toolsets-full-tools-multicluster-enum.json", tools.Tools)
		})
	})
}

func (s *ToolsetsSuite) TestGranularToolsetsTools() {
	testCases := []api.Toolset{
		&code.Toolset{},
		&core.Toolset{},
		&config.Toolset{},
		&helm.Toolset{},
		&kiali.Toolset{},
		&kubevirt.Toolset{},
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
				s.assertJsonSnapshot("toolsets-"+testCase.GetName()+"-tools.json", tools.Tools)
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
	// https://github.com/containers/kubernetes-mcp-server/issues/717
	// Verifies ALL tools have Properties initialized (not just cluster-aware ones)
	// OpenAI API requires properties field even if empty
	s.Run("InputSchema has properties for all tools regardless of cluster-awareness", func() {
		// Register all toolsets including kcp to catch kcp_workspaces_list
		toolsets.Clear()
		toolsets.Register(&core.Toolset{})
		toolsets.Register(&config.Toolset{})
		toolsets.Register(&helm.Toolset{})
		toolsets.Register(&kcp.Toolset{})
		s.Cfg.Toolsets = []string{"core", "config", "helm", "kcp"}
		// Enable multi-cluster mode to include configuration_contexts_list tool
		kubeconfig := s.Kubeconfig()
		kubeconfig.Contexts["extra-cluster"] = clientcmdapi.NewContext()
		s.Cfg.KubeConfig = test.KubeconfigFile(s.T(), kubeconfig)
		s.InitMcpClient()
		tools, err := s.ListTools(s.T().Context(), mcp.ListToolsRequest{})
		s.Require().NoError(err, "Expected no error from ListTools")
		s.Require().NotNil(tools, "Expected tools from ListTools")
		s.Require().NotEmpty(tools.Tools, "Expected at least one tool")
		// Check each tool has InputSchema.Properties initialized
		for _, tool := range tools.Tools {
			s.Run(tool.Name, func() {
				s.Require().NotNil(tool.InputSchema.Properties,
					"Expected InputSchema.Properties not to be nil for tool %s (required by OpenAI API)",
					tool.Name)
			})
		}
	})
}

func (s *ToolsetsSuite) InitMcpClient() {
	provider, err := kubernetes.NewProvider(s.Cfg)
	s.Require().NoError(err, "Expected no error creating kubernetes target provider")
	s.mcpServer, err = NewServer(Configuration{StaticConfig: s.Cfg}, provider)
	s.Require().NoError(err, "Expected no error creating MCP server")
	s.McpClient = test.NewMcpClient(s.T(), s.mcpServer.ServeHTTP())
}

// assertJsonSnapshot compares actual data against a JSON snapshot file.
// When the snapshot doesn't match, the test fails with instructions on how to update it.
// Set UPDATE_TOOLSETS_JSON=1 environment variable to regenerate snapshot files.
// Example: UPDATE_TOOLSETS_JSON=1 go test ./pkg/mcp -v
func (s *ToolsetsSuite) assertJsonSnapshot(snapshotFile string, actual any) {
	_, file, _, _ := runtime.Caller(1)
	snapshotPath := filepath.Join(filepath.Dir(file), "testdata", snapshotFile)
	actualJson, err := json.MarshalIndent(actual, "", "  ")
	s.Require().NoErrorf(err, "failed to marshal actual data: %v", err)
	if s.updateJson {
		err := os.WriteFile(snapshotPath, append(actualJson, '\n'), 0644)
		s.Require().NoErrorf(err, "failed to write snapshot file %s: %v", snapshotFile, err)
		s.T().Logf("Updated snapshot: %s", snapshotFile)
		return
	}
	expectedJson := test.ReadFile("testdata", snapshotFile)
	s.JSONEq(
		expectedJson,
		string(actualJson),
		"snapshot %s does not match - to update snapshots re-run the tests with %s=1",
		snapshotFile,
		updateJsonEnvVar,
	)
}

func TestToolsets(t *testing.T) {
	suite.Run(t, new(ToolsetsSuite))
}
