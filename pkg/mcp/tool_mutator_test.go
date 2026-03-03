package mcp

import (
	"context"
	"fmt"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"k8s.io/utils/ptr"
)

// createTestTool creates a basic ServerTool for testing
func createTestTool(name string) api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        name,
			Description: "A test tool",
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: make(map[string]*jsonschema.Schema),
			},
		},
	}
}

// createTestToolWithNilSchema creates a ServerTool with nil InputSchema for testing
func createTestToolWithNilSchema(name string) api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        name,
			Description: "A test tool",
			InputSchema: nil,
		},
	}
}

// createTestToolWithNilProperties creates a ServerTool with nil Properties for testing
func createTestToolWithNilProperties(name string) api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        name,
			Description: "A test tool",
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: nil,
			},
		},
	}
}

// createTestToolWithExistingProperties creates a ServerTool with existing properties for testing
func createTestToolWithExistingProperties(name string) api.ServerTool {
	return api.ServerTool{
		Tool: api.Tool{
			Name:        name,
			Description: "A test tool",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"existing-prop": {Type: "string"},
				},
			},
		},
	}
}

type mockProvider struct {
	targets []string
	err     error
}

func (m *mockProvider) IsOpenShift(_ context.Context) bool                              { return false }
func (m *mockProvider) IsMultiCluster() bool                                            { return len(m.targets) > 1 }
func (m *mockProvider) GetTargets(_ context.Context) ([]string, error)                  { return m.targets, m.err }
func (m *mockProvider) GetDerivedKubernetes(_ context.Context, _ string) (*internalk8s.Kubernetes, error) {
	return nil, nil
}
func (m *mockProvider) GetDefaultTarget() string            { return "" }
func (m *mockProvider) GetTargetParameterName() string      { return "" }
func (m *mockProvider) WatchTargets(_ internalk8s.McpReload) {}
func (m *mockProvider) Close()                              {}

func TestWithClusterParameter(t *testing.T) {
	tests := []struct {
		name                string
		defaultCluster      string
		targetParameterName string
		isMultiCluster      bool
		toolName            string
		toolFactory         func(string) api.ServerTool
		expectCluster       bool
	}{
		{
			name:           "adds cluster parameter when multi-cluster",
			defaultCluster: "default-cluster",
			isMultiCluster: true,
			toolName:       "test-tool",
			toolFactory:    createTestTool,
			expectCluster:  true,
		},
		{
			name:           "does not add cluster parameter when single cluster",
			defaultCluster: "default-cluster",
			isMultiCluster: false,
			toolName:       "test-tool",
			toolFactory:    createTestTool,
			expectCluster:  false,
		},
		{
			name:           "creates InputSchema when nil",
			defaultCluster: "default-cluster",
			isMultiCluster: true,
			toolName:       "test-tool",
			toolFactory:    createTestToolWithNilSchema,
			expectCluster:  true,
		},
		{
			name:           "creates Properties map when nil",
			defaultCluster: "default-cluster",
			isMultiCluster: true,
			toolName:       "test-tool",
			toolFactory:    createTestToolWithNilProperties,
			expectCluster:  true,
		},
		{
			name:           "preserves existing properties",
			defaultCluster: "default-cluster",
			isMultiCluster: true,
			toolName:       "test-tool",
			toolFactory:    createTestToolWithExistingProperties,
			expectCluster:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.targetParameterName == "" {
				tt.targetParameterName = "cluster"
			}
			mutator := WithTargetParameter(tt.defaultCluster, tt.targetParameterName, tt.isMultiCluster)
			tool := tt.toolFactory(tt.toolName)
			originalTool := tool // Keep reference to check if tool was unchanged

			result := mutator(tool)

			if !tt.expectCluster {
				if tt.toolName == "skip-this-tool" {
					// For skipped tools, the entire tool should be unchanged
					assert.Equal(t, originalTool, result)
				} else {
					// For single cluster, schema should exist but no cluster property
					require.NotNil(t, result.Tool.InputSchema)
					require.NotNil(t, result.Tool.InputSchema.Properties)
					_, exists := result.Tool.InputSchema.Properties["cluster"]
					assert.False(t, exists, "cluster property should not exist")
				}
				return
			}

			// Common assertions for cases where cluster parameter should be added
			require.NotNil(t, result.Tool.InputSchema)
			assert.Equal(t, "object", result.Tool.InputSchema.Type)
			require.NotNil(t, result.Tool.InputSchema.Properties)

			clusterProperty, exists := result.Tool.InputSchema.Properties["cluster"]
			assert.True(t, exists, "cluster property should exist")
			assert.NotNil(t, clusterProperty)
			assert.Equal(t, "string", clusterProperty.Type)
			assert.Contains(t, clusterProperty.Description, tt.defaultCluster)
		})
	}
}

func TestCreateClusterProperty(t *testing.T) {
	tests := []struct {
		name           string
		defaultCluster string
		targetName     string
	}{
		{
			name:           "creates property with correct type and description",
			defaultCluster: "default",
			targetName:     "cluster",
		},
		{
			name:           "includes default target in description",
			defaultCluster: "my-cluster",
			targetName:     "context",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			property := createTargetProperty(tt.defaultCluster, tt.targetName)

			assert.Equal(t, "string", property.Type)
			assert.Contains(t, property.Description, tt.defaultCluster)
			assert.Contains(t, property.Description, "Defaults to "+tt.defaultCluster+" if not set")
		})
	}
}

func TestToolMutatorType(t *testing.T) {
	t.Run("ToolMutator type can be used as function", func(t *testing.T) {
		var mutator ToolMutator = func(tool api.ServerTool) api.ServerTool {
			tool.Tool.Name = "modified-" + tool.Tool.Name
			return tool
		}

		originalTool := createTestTool("original")
		result := mutator(originalTool)
		assert.Equal(t, "modified-original", result.Tool.Name)
	})
}

type TargetParameterToolMutatorSuite struct {
	suite.Suite
}

func (s *TargetParameterToolMutatorSuite) TestClusterAwareTool() {
	tm := WithTargetParameter("default-cluster", "cluster", true)
	tool := createTestTool("cluster-aware-tool")
	// Tools are cluster-aware by default
	tm(tool)
	s.Require().NotNil(tool.Tool.InputSchema.Properties)
	s.Run("adds cluster parameter", func() {
		s.NotNil(tool.Tool.InputSchema.Properties["cluster"], "Expected cluster property to be added")
	})
	s.Run("adds correct description", func() {
		desc := tool.Tool.InputSchema.Properties["cluster"].Description
		s.Contains(desc, "Optional parameter selecting which cluster to run the tool in", "Expected description to mention cluster selection")
		s.Contains(desc, "Defaults to default-cluster if not set", "Expected description to mention default cluster")
	})
}

func (s *TargetParameterToolMutatorSuite) TestClusterAwareToolSingleCluster() {
	tm := WithTargetParameter("default", "cluster", false)
	tool := createTestTool("cluster-aware-tool-single-cluster")
	// Tools are cluster-aware by default
	tm(tool)
	s.Run("does not add cluster parameter for single cluster", func() {
		s.Nilf(tool.Tool.InputSchema.Properties["cluster"], "Expected cluster property to not be added for single cluster")
	})
}

func (s *TargetParameterToolMutatorSuite) TestClusterAwareToolMultipleClusters() {
	tm := WithTargetParameter("default", "cluster", true)
	tool := createTestTool("cluster-aware-tool-multiple-clusters")
	// Tools are cluster-aware by default
	tm(tool)
	s.Run("adds cluster parameter", func() {
		s.NotNilf(tool.Tool.InputSchema.Properties["cluster"], "Expected cluster property to be added")
	})
}

func (s *TargetParameterToolMutatorSuite) TestNonClusterAwareTool() {
	tm := WithTargetParameter("default", "cluster", true)
	tool := createTestTool("non-cluster-aware-tool")
	tool.ClusterAware = ptr.To(false)
	tm(tool)
	s.Run("does not add cluster parameter", func() {
		s.Nilf(tool.Tool.InputSchema.Properties["cluster"], "Expected cluster property to not be added")
	})
}

func TestTargetParameterToolMutator(t *testing.T) {
	suite.Run(t, new(TargetParameterToolMutatorSuite))
}

type TargetListToolMutatorSuite struct {
	suite.Suite
}

func (s *TargetListToolMutatorSuite) TestMutatesTargetsListTool() {
	tool := createTestTool(TargetsListToolName)
	tm := WithTargetListTool("default-cluster", "cluster", &mockProvider{targets: []string{"cluster-1", "cluster-2", "cluster-3"}})
	result := tm(tool)

	s.Run("renames tool based on target parameter", func() {
		s.Equal("cluster_list", result.Tool.Name)
	})
	s.Run("updates description", func() {
		s.Contains(result.Tool.Description, "cluster")
		s.Contains(result.Tool.Description, "List all available")
	})
	s.Run("updates title annotation", func() {
		s.Equal("Cluster List", result.Tool.Annotations.Title)
	})
	s.Run("sets handler", func() {
		s.NotNil(result.Handler)
	})
}

func (s *TargetListToolMutatorSuite) TestDoesNotMutateOtherTools() {
	tool := createTestTool("some-other-tool")
	tm := WithTargetListTool("default", "cluster", &mockProvider{targets: []string{"cluster-1", "cluster-2"}})
	result := tm(tool)

	s.Equal("some-other-tool", result.Tool.Name, "tool name should remain unchanged")
}

func (s *TargetListToolMutatorSuite) TestHandlerWithEmptyTargets() {
	tool := createTestTool(TargetsListToolName)
	tm := WithTargetListTool("default", "cluster", &mockProvider{targets: []string{}})
	result := tm(tool)

	s.Require().NotNil(result.Handler)
	callResult, err := result.Handler(api.ToolHandlerParams{Context: context.Background()})
	s.NoError(err)
	s.Contains(callResult.Content, "No clusters available")
}

func (s *TargetListToolMutatorSuite) TestHandlerWithGetTargetsError() {
	tool := createTestTool(TargetsListToolName)
	tm := WithTargetListTool("default", "cluster", &mockProvider{err: fmt.Errorf("unauthorized")})
	result := tm(tool)

	s.Require().NotNil(result.Handler)
	callResult, err := result.Handler(api.ToolHandlerParams{Context: context.Background()})
	s.NoError(err)
	s.Require().NotNil(callResult.Error)
	s.Contains(callResult.Error.Error(), "failed to find any targets")
	s.Contains(callResult.Error.Error(), "unauthorized")
}

func TestTargetListToolMutator(t *testing.T) {
	suite.Run(t, new(TargetListToolMutatorSuite))
}
