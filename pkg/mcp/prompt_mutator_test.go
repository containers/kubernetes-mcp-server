package mcp

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"k8s.io/utils/ptr"
)

func createTestPrompt(name string) api.ServerPrompt {
	return api.ServerPrompt{
		Prompt: api.Prompt{
			Name:        name,
			Description: "A test prompt",
			Arguments: []api.PromptArgument{
				{
					Name:        "namespace",
					Description: "Optional namespace",
					Required:    false,
				},
			},
		},
	}
}

func TestPromptMutatorType(t *testing.T) {
	t.Run("PromptMutator type can be used as function", func(t *testing.T) {
		var mutator PromptMutator = func(prompt api.ServerPrompt) api.ServerPrompt {
			prompt.Prompt.Name = "modified-" + prompt.Prompt.Name
			return prompt
		}

		original := createTestPrompt("original")
		result := mutator(original)
		assert.Equal(t, "modified-original", result.Prompt.Name)
	})
}

type TargetParameterPromptMutatorSuite struct {
	suite.Suite
}

func (s *TargetParameterPromptMutatorSuite) TestClusterAwarePrompt() {
	pm := WithPromptTargetParameter("default-cluster", "context", true)
	prompt := pm(createTestPrompt("cluster-aware-prompt"))
	s.Require().Len(prompt.Prompt.Arguments, 2)
	s.Run("adds context argument", func() {
		arg := prompt.Prompt.Arguments[1]
		s.Equal("context", arg.Name)
		s.False(arg.Required)
	})
	s.Run("adds correct description", func() {
		desc := prompt.Prompt.Arguments[1].Description
		s.Contains(desc, "Optional parameter selecting which context to run the prompt in")
		s.Contains(desc, "Defaults to default-cluster if not set")
	})
	s.Run("preserves existing arguments", func() {
		s.Equal("namespace", prompt.Prompt.Arguments[0].Name)
	})
}

func (s *TargetParameterPromptMutatorSuite) TestClusterAwarePromptSingleCluster() {
	pm := WithPromptTargetParameter("default", "context", false)
	prompt := pm(createTestPrompt("cluster-aware-prompt-single-cluster"))
	s.Run("does not add context argument for single cluster", func() {
		s.Len(prompt.Prompt.Arguments, 1, "Expected only the original argument")
	})
}

func (s *TargetParameterPromptMutatorSuite) TestNonClusterAwarePrompt() {
	nonClusterAware := createTestPrompt("non-cluster-aware-prompt")
	nonClusterAware.ClusterAware = ptr.To(false)
	pm := WithPromptTargetParameter("default", "context", true)
	prompt := pm(nonClusterAware)
	s.Run("does not add context argument", func() {
		s.Len(prompt.Prompt.Arguments, 1, "Expected only the original argument")
	})
}

func TestTargetParameterPromptMutator(t *testing.T) {
	suite.Run(t, new(TargetParameterPromptMutatorSuite))
}
