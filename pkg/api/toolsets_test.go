package api

import (
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

func TestToolsets(t *testing.T) {
	suite.Run(t, new(ToolsetsSuite))
}
