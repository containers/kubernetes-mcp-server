package api

import (
	"encoding/json"
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
