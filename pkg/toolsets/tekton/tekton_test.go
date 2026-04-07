package tekton_test

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/tekton"
	"github.com/stretchr/testify/suite"
)

type TektonSuite struct {
	suite.Suite
}

func TestTekton(t *testing.T) {
	suite.Run(t, new(TektonSuite))
}

func (s *TektonSuite) TestToolset() {
	ts := &tekton.Toolset{}
	s.Equal("tekton", ts.GetName())
	s.NotEmpty(ts.GetDescription())
	tools := ts.GetTools(nil)
	s.NotEmpty(tools)
	s.Nil(ts.GetPrompts())
}
