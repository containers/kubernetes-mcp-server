package mcp

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		skipToolNames       []string
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
			skipToolNames:  []string{},
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
			skipToolNames:  []string{},
			toolName:       "test-tool",
			toolFactory:    createTestTool,
			expectCluster:  false,
			expectEnum:     false,
			enumCount:      0,
		},
		{
			name:           "skips tools in skipToolNames list",
			defaultCluster: "default-cluster",
			clusters:       []string{"cluster1", "cluster2"},
			skipToolNames:  []string{"skip-this-tool"},
			toolName:       "skip-this-tool",
			toolFactory:    createTestTool,
			expectCluster:  false,
			expectEnum:     false,
			enumCount:      0,
		},
		{
			name:           "creates InputSchema when nil",
			defaultCluster: "default-cluster",
			clusters:       []string{"cluster1", "cluster2"},
			skipToolNames:  []string{},
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
			skipToolNames:  []string{},
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
			skipToolNames:  []string{},
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
			mutator := WithTargetParameter(tt.defaultCluster, tt.targetParameterName, tt.clusters, tt.skipToolNames)
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
