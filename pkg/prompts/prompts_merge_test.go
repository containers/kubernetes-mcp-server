package prompts

import (
	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func (s *PromptsTestSuite) TestMergePrompts() {
	s.Run("with no overlap, prompts are combined", func() {
		base := []api.ServerPrompt{
			{Prompt: api.Prompt{Name: "prompt1"}},
			{Prompt: api.Prompt{Name: "prompt2"}},
		}
		override := []api.ServerPrompt{
			{Prompt: api.Prompt{Name: "prompt3"}},
		}

		result := MergePrompts(base, override)

		s.Len(result, 3)
		s.Equal("prompt1", result[0].Prompt.Name)
		s.Equal("prompt2", result[1].Prompt.Name)
		s.Equal("prompt3", result[2].Prompt.Name)
	})

	s.Run("with overlap, override prompt replaces base prompt", func() {
		base := []api.ServerPrompt{
			{Prompt: api.Prompt{Name: "prompt1", Description: "Base 1 description"}},
			{Prompt: api.Prompt{Name: "prompt2", Description: "Base 2 description"}},
		}
		override := []api.ServerPrompt{
			{Prompt: api.Prompt{Name: "prompt1", Description: "Override 1 description"}},
		}

		result := MergePrompts(base, override)

		s.Len(result, 2)
		s.Equal("prompt2", result[0].Prompt.Name)
		s.Equal("Base 2 description", result[0].Prompt.Description)
		s.Equal("prompt1", result[1].Prompt.Name)
		s.Equal("Override 1 description", result[1].Prompt.Description)
	})
	s.Run("with empty base, override prompts are used", func() {
		var base []api.ServerPrompt
		override := []api.ServerPrompt{
			{Prompt: api.Prompt{Name: "prompt1"}},
			{Prompt: api.Prompt{Name: "prompt2"}},
		}

		result := MergePrompts(base, override)

		s.Len(result, 2)
		s.Equal("prompt1", result[0].Prompt.Name)
		s.Equal("prompt2", result[1].Prompt.Name)
	})
	s.Run("with empty override, base prompts are used", func() {
		base := []api.ServerPrompt{
			{Prompt: api.Prompt{Name: "prompt1"}},
			{Prompt: api.Prompt{Name: "prompt2"}},
		}
		var override []api.ServerPrompt

		result := MergePrompts(base, override)

		s.Len(result, 2)
		s.Equal("prompt1", result[0].Prompt.Name)
		s.Equal("prompt2", result[1].Prompt.Name)
	})
	s.Run("with both empty, result is empty", func() {
		var base []api.ServerPrompt
		var override []api.ServerPrompt

		result := MergePrompts(base, override)

		s.Len(result, 0)
	})
	s.Run("with multiple overrides", func() {
		base := []api.ServerPrompt{
			{Prompt: api.Prompt{Name: "prompt1", Description: "Base 1"}},
			{Prompt: api.Prompt{Name: "prompt2", Description: "Base 2"}},
			{Prompt: api.Prompt{Name: "prompt3", Description: "Base 3"}},
		}
		override := []api.ServerPrompt{
			{Prompt: api.Prompt{Name: "prompt1", Description: "Override 1"}},
			{Prompt: api.Prompt{Name: "prompt3", Description: "Override 3"}},
		}

		result := MergePrompts(base, override)

		s.Len(result, 3)
		s.Equal("prompt2", result[0].Prompt.Name)
		s.Equal("Base 2", result[0].Prompt.Description)
		s.Equal("prompt1", result[1].Prompt.Name)
		s.Equal("Override 1", result[1].Prompt.Description)
		s.Equal("prompt3", result[2].Prompt.Name)
		s.Equal("Override 3", result[2].Prompt.Description)
	})
	s.Run("base prompts come first, then overrides", func() {
		base := []api.ServerPrompt{
			{Prompt: api.Prompt{Name: "base1"}},
			{Prompt: api.Prompt{Name: "base2"}},
			{Prompt: api.Prompt{Name: "base3"}},
		}

		override := []api.ServerPrompt{
			{Prompt: api.Prompt{Name: "override1"}},
			{Prompt: api.Prompt{Name: "override2"}},
		}

		result := MergePrompts(base, override)

		// Base prompts should come first (those not overridden)
		s.Equal("base1", result[0].Prompt.Name)
		s.Equal("base2", result[1].Prompt.Name)
		s.Equal("base3", result[2].Prompt.Name)

		// Then override prompts
		s.Equal("override1", result[3].Prompt.Name)
		s.Equal("override2", result[4].Prompt.Name)
	})
}

func (s *PromptsTestSuite) TestMergePromptsCompleteReplacement() {
	base := []api.ServerPrompt{
		{
			Prompt: api.Prompt{
				Name:        "test-prompt",
				Description: "Base description",
				Arguments: []api.PromptArgument{
					{Name: "base_arg", Required: true},
				},
			},
		},
	}

	override := []api.ServerPrompt{
		{
			Prompt: api.Prompt{
				Name:        "test-prompt",
				Description: "Override description",
				Arguments: []api.PromptArgument{
					{Name: "override_arg", Required: false},
				},
			},
		},
	}

	result := MergePrompts(base, override)

	s.Len(result, 1)
	s.Equal("test-prompt", result[0].Prompt.Name)
	s.Equal("Override description", result[0].Prompt.Description)
	s.Len(result[0].Prompt.Arguments, 1)
	s.Equal("override_arg", result[0].Prompt.Arguments[0].Name)
	s.False(result[0].Prompt.Arguments[0].Required)
}
