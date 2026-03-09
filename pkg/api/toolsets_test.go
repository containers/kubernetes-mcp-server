package api

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	"k8s.io/utils/ptr"
)

type ToolsetsSuite struct {
	suite.Suite
}

func (s *ToolsetsSuite) TestServerTool() {
	s.Run("IsClusterAware", func() {
		s.Run("defaults to true", func() {
			tool := &ServerTool{}
			s.True(tool.IsClusterAware(), "Expected IsClusterAware to be true by default")
		})
		s.Run("can be set to false", func() {
			tool := &ServerTool{ClusterAware: ptr.To(false)}
			s.False(tool.IsClusterAware(), "Expected IsClusterAware to be false when set to false")
		})
		s.Run("can be set to true", func() {
			tool := &ServerTool{ClusterAware: ptr.To(true)}
			s.True(tool.IsClusterAware(), "Expected IsClusterAware to be true when set to true")
		})
	})
	s.Run("IsTargetListProvider", func() {
		s.Run("defaults to false", func() {
			tool := &ServerTool{}
			s.False(tool.IsTargetListProvider(), "Expected IsTargetListProvider to be false by default")
		})
		s.Run("can be set to false", func() {
			tool := &ServerTool{TargetListProvider: ptr.To(false)}
			s.False(tool.IsTargetListProvider(), "Expected IsTargetListProvider to be false when set to false")
		})
		s.Run("can be set to true", func() {
			tool := &ServerTool{TargetListProvider: ptr.To(true)}
			s.True(tool.IsTargetListProvider(), "Expected IsTargetListProvider to be true when set to true")
		})
	})
}

func (s *ToolsetsSuite) TestNewToolCallResult() {
	s.Run("sets content and nil error", func() {
		result := NewToolCallResult("output text", nil)
		s.Equal("output text", result.Content)
		s.Nil(result.Error)
		s.Nil(result.StructuredContent)
	})
	s.Run("sets content and error", func() {
		err := errors.New("something failed")
		result := NewToolCallResult("partial output", err)
		s.Equal("partial output", result.Content)
		s.Equal(err, result.Error)
		s.Nil(result.StructuredContent)
	})
	s.Run("leaves StructuredContent nil", func() {
		result := NewToolCallResult("text", nil)
		s.Nil(result.StructuredContent)
	})
}

func (s *ToolsetsSuite) TestNewToolCallResultStructured() {
	s.Run("sets empty content when structured is nil", func() {
		result := NewToolCallResultStructured(nil, nil)
		s.Equal("", result.Content)
		s.Nil(result.StructuredContent)
	})
	s.Run("sets error and structured content", func() {
		err := errors.New("partial failure")
		structured := map[string]any{"key": "value"}
		result := NewToolCallResultStructured(structured, err)
		s.Equal(`{"key":"value"}`, result.Content)
		s.Equal(err, result.Error)
		s.Equal(structured, result.StructuredContent)
	})
	s.Run("handles complex nested structures", func() {
		structured := map[string]any{
			"metadata": map[string]any{"name": "test-pod"},
			"items":    []int{1, 2, 3},
		}
		result := NewToolCallResultStructured(structured, nil)
		s.Contains(result.Content, `"metadata"`)
		s.Contains(result.Content, `"name":"test-pod"`)
		s.Equal(structured, result.StructuredContent)
	})
	// Per MCP spec: "For backwards compatibility, a tool that returns structured content
	// SHOULD also return the serialized JSON in a TextContent block."
	// https://modelcontextprotocol.io/specification/2025-11-25/server/tools#structured-content
	s.Run("Content field contains JSON serialization of StructuredContent for MCP backward compatibility", func() {
		structured := map[string]any{"pods": []string{"pod-1", "pod-2"}, "count": 2}
		result := NewToolCallResultStructured(structured, nil)

		// Content should be valid JSON that represents the same data as StructuredContent
		s.JSONEq(`{"pods":["pod-1","pod-2"],"count":2}`, result.Content)
		// StructuredContent should be the original value
		s.Equal(structured, result.StructuredContent)
		s.Nil(result.Error)
	})
}

func (s *ToolsetsSuite) TestToolMeta() {
	s.Run("Meta is omitted from JSON when nil", func() {
		tool := Tool{Name: "test_tool"}
		b, err := json.Marshal(tool)
		s.Require().NoError(err)
		s.NotContains(string(b), "_meta")
	})
	s.Run("Meta is included in JSON when set", func() {
		tool := Tool{
			Name: "test_tool",
			Meta: map[string]any{
				"ui": map[string]any{
					"resourceUri": "ui://server/app.html",
				},
			},
		}
		b, err := json.Marshal(tool)
		s.Require().NoError(err)
		s.Contains(string(b), `"_meta"`)
		s.Contains(string(b), `"resourceUri"`)

		var parsed map[string]any
		s.Require().NoError(json.Unmarshal(b, &parsed))
		meta, ok := parsed["_meta"].(map[string]any)
		s.Require().True(ok, "expected _meta to be a map")
		ui, ok := meta["ui"].(map[string]any)
		s.Require().True(ok, "expected ui to be a map")
		s.Equal("ui://server/app.html", ui["resourceUri"])
	})
}

func TestToolsets(t *testing.T) {
	suite.Run(t, new(ToolsetsSuite))
}
