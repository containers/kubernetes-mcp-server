package argocd_test

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/argocd"
	"github.com/stretchr/testify/suite"
)

type ArgocdSuite struct {
	suite.Suite
}

func TestArgocd(t *testing.T) {
	suite.Run(t, new(ArgocdSuite))
}

func (s *ArgocdSuite) TestToolset() {
	ts := &argocd.Toolset{}
	s.Equal("argocd", ts.GetName())
	s.NotEmpty(ts.GetDescription())
	tools := ts.GetTools(nil)
	s.Len(tools, 6)
	s.Nil(ts.GetPrompts())
	s.Nil(ts.GetResources())
	s.Nil(ts.GetResourceTemplates())
}
