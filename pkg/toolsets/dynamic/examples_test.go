package dynamic

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
	"sigs.k8s.io/yaml"
)

// ExamplesSuite keeps the Tool definitions shipped under examples/dynamic-tools
// valid: every file must parse, validate and compile its CEL expressions.
type ExamplesSuite struct {
	suite.Suite
}

func (s *ExamplesSuite) TestExampleToolsAreValid() {
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "examples", "dynamic-tools", "tools", "*.yaml"))
	s.Require().NoError(err)
	s.Require().NotEmpty(files, "expected example Tool definitions")
	for _, file := range files {
		s.Run(filepath.Base(file), func() {
			raw, err := os.ReadFile(file)
			s.Require().NoError(err)
			var tool Tool
			s.Require().NoError(yaml.Unmarshal(raw, &tool))
			s.Equal(Group+"/"+Version, mustString(raw, "apiVersion"))
			compiled, err := newCompiledTool(tool)
			s.Require().NoError(err)
			serverTool, err := compiled.serverTool(s.T().Context(), nil)
			s.Require().NoError(err)
			s.NotEmpty(serverTool.Tool.Name)
			s.NotEmpty(serverTool.Tool.Description)
			s.NotNil(serverTool.RBAC)
		})
	}
}

func mustString(raw []byte, key string) string {
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return ""
	}
	value, _ := doc[key].(string)
	return value
}

func TestExamples(t *testing.T) {
	suite.Run(t, new(ExamplesSuite))
}
