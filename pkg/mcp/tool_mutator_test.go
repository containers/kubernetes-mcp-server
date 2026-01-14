package mcp

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
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

func TestWithClusterParameter(t *testing.T) {
	tests := []struct {
		name                string
		defaultCluster      string
		targetParameterName string
		clusters            []string
		toolName            string
		toolFactory         func(string) api.ServerTool
		expectCluster       bool
		expectEnum          bool
		enumCount           int
	}{
		{
			name:           "adds cluster parameter when multiple clusters provided",
			defaultCluster: "default-cluster",
			clusters:       []string{"cluster1", "cluster2", "cluster3"},
			toolName:       "test-tool",
			toolFactory:    createTestTool,
			expectCluster:  true,
			expectEnum:     true,
			enumCount:      3,
		},
		{
			name:           "does not add cluster parameter when single cluster provided",
			defaultCluster: "default-cluster",
			clusters:       []string{"single-cluster"},
			toolName:       "test-tool",
			toolFactory:    createTestTool,
			expectCluster:  false,
			expectEnum:     false,
			enumCount:      0,
		},
		{
			name:           "creates InputSchema when nil",
			defaultCluster: "default-cluster",
			clusters:       []string{"cluster1", "cluster2"},
			toolName:       "test-tool",
			toolFactory:    createTestToolWithNilSchema,
			expectCluster:  true,
			expectEnum:     true,
			enumCount:      2,
		},
		{
			name:           "creates Properties map when nil",
			defaultCluster: "default-cluster",
			clusters:       []string{"cluster1", "cluster2"},
			toolName:       "test-tool",
			toolFactory:    createTestToolWithNilProperties,
			expectCluster:  true,
			expectEnum:     true,
			enumCount:      2,
		},
		{
			name:           "preserves existing properties",
			defaultCluster: "default-cluster",
			clusters:       []string{"cluster1", "cluster2"},
			toolName:       "test-tool",
			toolFactory:    createTestToolWithExistingProperties,
			expectCluster:  true,
			expectEnum:     true,
			enumCount:      2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.targetParameterName == "" {
				tt.targetParameterName = "cluster"
			}
			mutator := WithTargetParameter(tt.defaultCluster, tt.targetParameterName, tt.clusters)
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

			if tt.expectEnum {
				assert.NotNil(t, clusterProperty.Enum)
				assert.Equal(t, tt.enumCount, len(clusterProperty.Enum))
				for _, cluster := range tt.clusters {
					assert.Contains(t, clusterProperty.Enum, cluster)
				}
			}
		})
	}
}

func TestCreateClusterProperty(t *testing.T) {
	tests := []struct {
		name           string
		defaultCluster string
		targetName     string
		clusters       []string
		expectEnum     bool
		expectedCount  int
	}{
		{
			name:           "creates property with enum when clusters <= maxClustersInEnum",
			defaultCluster: "default",
			targetName:     "cluster",
			clusters:       []string{"cluster1", "cluster2", "cluster3"},
			expectEnum:     true,
			expectedCount:  3,
		},
		{
			name:           "creates property without enum when clusters > maxClustersInEnum",
			defaultCluster: "default",
			targetName:     "cluster",
			clusters:       make([]string, maxTargetsInEnum+5), // 20 clusters
			expectEnum:     false,
			expectedCount:  0,
		},
		{
			name:           "creates property with exact maxClustersInEnum clusters",
			defaultCluster: "default",
			targetName:     "cluster",
			clusters:       make([]string, maxTargetsInEnum),
			expectEnum:     true,
			expectedCount:  maxTargetsInEnum,
		},
		{
			name:           "handles single cluster",
			defaultCluster: "default",
			targetName:     "cluster",
			clusters:       []string{"single-cluster"},
			expectEnum:     true,
			expectedCount:  1,
		},
		{
			name:           "handles empty clusters list",
			defaultCluster: "default",
			targetName:     "cluster",
			clusters:       []string{},
			expectEnum:     true,
			expectedCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize clusters with names if they were created with make()
			if len(tt.clusters) > 3 && tt.clusters[0] == "" {
				for i := range tt.clusters {
					tt.clusters[i] = "cluster" + string(rune('A'+i))
				}
			}

			property := createTargetProperty(tt.defaultCluster, tt.targetName, tt.clusters)

			assert.Equal(t, "string", property.Type)
			assert.Contains(t, property.Description, tt.defaultCluster)
			assert.Contains(t, property.Description, "Defaults to "+tt.defaultCluster+" if not set")

			if tt.expectEnum {
				assert.NotNil(t, property.Enum, "enum should be created")
				assert.Equal(t, tt.expectedCount, len(property.Enum))
				if tt.expectedCount > 0 && tt.expectedCount <= 3 {
					// Only check specific values for small, predefined lists
					for _, cluster := range tt.clusters {
						assert.Contains(t, property.Enum, cluster)
					}
				}
			} else {
				assert.Nil(t, property.Enum, "enum should not be created for too many clusters")
			}
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

func TestMaxClustersInEnumConstant(t *testing.T) {
	t.Run("maxClustersInEnum has expected value", func(t *testing.T) {
		assert.Equal(t, 5, maxTargetsInEnum, "maxClustersInEnum should be 5")
	})
}

type TargetParameterToolMutatorSuite struct {
	suite.Suite
}

func (s *TargetParameterToolMutatorSuite) TestClusterAwareTool() {
	tm := WithTargetParameter("default-cluster", "cluster", []string{"cluster-1", "cluster-2", "cluster-3"})
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
	s.Run("adds enum with clusters", func() {
		s.Require().NotNil(tool.Tool.InputSchema.Properties["cluster"])
		enum := tool.Tool.InputSchema.Properties["cluster"].Enum
		s.NotNilf(enum, "Expected enum to be set")
		s.Equal(3, len(enum), "Expected enum to have 3 entries")
		s.Contains(enum, "cluster-1", "Expected enum to contain cluster-1")
		s.Contains(enum, "cluster-2", "Expected enum to contain cluster-2")
		s.Contains(enum, "cluster-3", "Expected enum to contain cluster-3")
	})
}

func (s *TargetParameterToolMutatorSuite) TestClusterAwareToolSingleCluster() {
	tm := WithTargetParameter("default", "cluster", []string{"only-cluster"})
	tool := createTestTool("cluster-aware-tool-single-cluster")
	// Tools are cluster-aware by default
	tm(tool)
	s.Run("does not add cluster parameter for single cluster", func() {
		s.Nilf(tool.Tool.InputSchema.Properties["cluster"], "Expected cluster property to not be added for single cluster")
	})
}

func (s *TargetParameterToolMutatorSuite) TestClusterAwareToolMultipleClusters() {
	tm := WithTargetParameter("default", "cluster", []string{"cluster-1", "cluster-2", "cluster-3", "cluster-4", "cluster-5", "cluster-6"})
	tool := createTestTool("cluster-aware-tool-multiple-clusters")
	// Tools are cluster-aware by default
	tm(tool)
	s.Run("adds cluster parameter", func() {
		s.NotNilf(tool.Tool.InputSchema.Properties["cluster"], "Expected cluster property to be added")
	})
	s.Run("does not add enum when list of clusters is > 5", func() {
		s.Require().NotNil(tool.Tool.InputSchema.Properties["cluster"])
		enum := tool.Tool.InputSchema.Properties["cluster"].Enum
		s.Nilf(enum, "Expected enum to not be set for too many clusters")
	})
}

func (s *TargetParameterToolMutatorSuite) TestNonClusterAwareTool() {
	tm := WithTargetParameter("default", "cluster", []string{"cluster-1", "cluster-2"})
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
	tm := WithTargetListTool("default-cluster", "cluster", []string{"cluster-1", "cluster-2", "cluster-3"})
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
	tm := WithTargetListTool("default", "cluster", []string{"cluster-1", "cluster-2"})
	result := tm(tool)

	s.Equal("some-other-tool", result.Tool.Name, "tool name should remain unchanged")
}

func (s *TargetListToolMutatorSuite) TestHandlerWithEmptyTargets() {
	tool := createTestTool(TargetsListToolName)
	tm := WithTargetListTool("default", "cluster", []string{})
	result := tm(tool)

	s.Require().NotNil(result.Handler)
	callResult, err := result.Handler(api.ToolHandlerParams{})
	s.NoError(err)
	s.Contains(callResult.Content, "No clusters available")
}

func TestTargetListToolMutator(t *testing.T) {
	suite.Run(t, new(TargetListToolMutatorSuite))
}
