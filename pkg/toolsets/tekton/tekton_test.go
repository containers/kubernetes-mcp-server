package tekton_test

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/tekton"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolset(t *testing.T) {
	ts := &tekton.Toolset{}
	assert.Equal(t, "tekton", ts.GetName())
	assert.NotEmpty(t, ts.GetDescription())
	tools := ts.GetTools(nil)
	assert.NotEmpty(t, tools)
	assert.Nil(t, ts.GetPrompts())
}

func TestToolNames(t *testing.T) {
	tools := (&tekton.Toolset{}).GetTools(nil)
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Tool.Name)
	}
	expected := []string{
		"tekton_pipeline_start",
		"tekton_pipelinerun_restart",
		"tekton_task_start",
		"tekton_taskrun_restart",
		"tekton_taskrun_logs",
	}
	assert.ElementsMatch(t, expected, names)
}

func TestToolCount(t *testing.T) {
	tools := (&tekton.Toolset{}).GetTools(nil)
	assert.Len(t, tools, 5)
}

func TestToolSchemas(t *testing.T) {
	tools := (&tekton.Toolset{}).GetTools(nil)
	for _, tool := range tools {
		t.Run(tool.Tool.Name, func(t *testing.T) {
			require.NotNil(t, tool.Tool.InputSchema, "tool %s must have an input schema", tool.Tool.Name)
			assert.Equal(t, "object", tool.Tool.InputSchema.Type, "tool %s schema must be object type", tool.Tool.Name)
			assert.NotEmpty(t, tool.Tool.InputSchema.Properties, "tool %s must have schema properties", tool.Tool.Name)
		})
	}
}

func TestReadOnlyTools(t *testing.T) {
	tools := (&tekton.Toolset{}).GetTools(nil)
	readOnlyTools := map[string]bool{
		"tekton_taskrun_logs": true,
	}
	for _, tool := range tools {
		t.Run(tool.Tool.Name, func(t *testing.T) {
			if readOnlyTools[tool.Tool.Name] {
				require.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
				assert.True(t, *tool.Tool.Annotations.ReadOnlyHint, "tool %s should be read-only", tool.Tool.Name)
				require.NotNil(t, tool.Tool.Annotations.DestructiveHint)
				assert.False(t, *tool.Tool.Annotations.DestructiveHint, "read-only tool %s should not be destructive", tool.Tool.Name)
			}
		})
	}
}


func TestRequiredParameters(t *testing.T) {
	tools := (&tekton.Toolset{}).GetTools(nil)
	requiredParams := map[string][]string{
		"tekton_pipeline_start":      {"name"},
		"tekton_pipelinerun_restart": {"name"},
		"tekton_task_start":          {"name"},
		"tekton_taskrun_restart":     {"name"},
		"tekton_taskrun_logs":        {"name"},
	}
	for _, tool := range tools {
		t.Run(tool.Tool.Name, func(t *testing.T) {
			expected, ok := requiredParams[tool.Tool.Name]
			require.True(t, ok, "unexpected tool: %s", tool.Tool.Name)
			assert.ElementsMatch(t, expected, tool.Tool.InputSchema.Required,
				"tool %s required params mismatch", tool.Tool.Name)
		})
	}
}

func TestToolAnnotations(t *testing.T) {
	tools := (&tekton.Toolset{}).GetTools(nil)
	for _, tool := range tools {
		t.Run(tool.Tool.Name, func(t *testing.T) {
			assert.NotEmpty(t, tool.Tool.Annotations.Title, "tool %s must have a title", tool.Tool.Name)
			require.NotNil(t, tool.Tool.Annotations.DestructiveHint, "tool %s must have DestructiveHint set", tool.Tool.Name)
		})
	}
}

func TestHandlersNotNil(t *testing.T) {
	tools := (&tekton.Toolset{}).GetTools(nil)
	for _, tool := range tools {
		t.Run(tool.Tool.Name, func(t *testing.T) {
			assert.NotNil(t, tool.Handler, "tool %s must have a handler", tool.Tool.Name)
		})
	}
}

func TestStartToolsHaveParamsProperty(t *testing.T) {
	tools := (&tekton.Toolset{}).GetTools(nil)
	startTools := map[string]bool{
		"tekton_pipeline_start": true,
		"tekton_task_start":     true,
	}
	for _, tool := range tools {
		if !startTools[tool.Tool.Name] {
			continue
		}
		t.Run(tool.Tool.Name, func(t *testing.T) {
			require.NotNil(t, tool.Tool.InputSchema)
			paramsSchema, ok := tool.Tool.InputSchema.Properties["params"]
			require.True(t, ok, "tool %s must have a 'params' property", tool.Tool.Name)
			assert.Equal(t, "object", paramsSchema.Type, "params property must be of type object")
			assert.NotNil(t, paramsSchema.AdditionalProperties, "params property must allow additional properties")
			assert.NotEmpty(t, paramsSchema.Description, "params property must have a description")
		})
	}
}
