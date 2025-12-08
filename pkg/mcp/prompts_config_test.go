package mcp

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/assert"
)

func TestMergePrompts(t *testing.T) {
	tests := []struct {
		name           string
		base           []api.ServerPrompt
		override       []api.ServerPrompt
		expectedCount  int
		expectedNames  []string
		expectedSource string // Which source should win for overlapping names
	}{
		{
			name: "merge with no overlap",
			base: []api.ServerPrompt{
				{Prompt: api.Prompt{Name: "prompt1"}},
				{Prompt: api.Prompt{Name: "prompt2"}},
			},
			override: []api.ServerPrompt{
				{Prompt: api.Prompt{Name: "prompt3"}},
			},
			expectedCount: 3,
			expectedNames: []string{"prompt1", "prompt2", "prompt3"},
		},
		{
			name: "override replaces base prompt with same name",
			base: []api.ServerPrompt{
				{Prompt: api.Prompt{Name: "prompt1", Description: "Base description"}},
				{Prompt: api.Prompt{Name: "prompt2"}},
			},
			override: []api.ServerPrompt{
				{Prompt: api.Prompt{Name: "prompt1", Description: "Override description"}},
			},
			expectedCount:  2,
			expectedNames:  []string{"prompt2", "prompt1"},
			expectedSource: "Override description",
		},
		{
			name: "empty base",
			base: []api.ServerPrompt{},
			override: []api.ServerPrompt{
				{Prompt: api.Prompt{Name: "prompt1"}},
			},
			expectedCount: 1,
			expectedNames: []string{"prompt1"},
		},
		{
			name: "empty override",
			base: []api.ServerPrompt{
				{Prompt: api.Prompt{Name: "prompt1"}},
			},
			override:      []api.ServerPrompt{},
			expectedCount: 1,
			expectedNames: []string{"prompt1"},
		},
		{
			name:          "both empty",
			base:          []api.ServerPrompt{},
			override:      []api.ServerPrompt{},
			expectedCount: 0,
		},
		{
			name: "multiple overrides",
			base: []api.ServerPrompt{
				{Prompt: api.Prompt{Name: "prompt1", Description: "Base 1"}},
				{Prompt: api.Prompt{Name: "prompt2", Description: "Base 2"}},
				{Prompt: api.Prompt{Name: "prompt3", Description: "Base 3"}},
			},
			override: []api.ServerPrompt{
				{Prompt: api.Prompt{Name: "prompt1", Description: "Override 1"}},
				{Prompt: api.Prompt{Name: "prompt3", Description: "Override 3"}},
			},
			expectedCount:  3,
			expectedNames:  []string{"prompt2", "prompt1", "prompt3"},
			expectedSource: "Override 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergePrompts(tt.base, tt.override)

			assert.Len(t, result, tt.expectedCount, "unexpected number of prompts")

			if len(tt.expectedNames) > 0 {
				actualNames := make([]string, len(result))
				for i, p := range result {
					actualNames[i] = p.Prompt.Name
				}
				assert.ElementsMatch(t, tt.expectedNames, actualNames, "prompt names don't match")
			}

			// Check that override wins for specific test case
			if tt.expectedSource != "" {
				for _, p := range result {
					if p.Prompt.Name == "prompt1" {
						assert.Equal(t, tt.expectedSource, p.Prompt.Description, "override should win")
					}
				}
			}
		})
	}
}

func TestMergePromptsPreservesOrder(t *testing.T) {
	base := []api.ServerPrompt{
		{Prompt: api.Prompt{Name: "base1"}},
		{Prompt: api.Prompt{Name: "base2"}},
		{Prompt: api.Prompt{Name: "base3"}},
	}

	override := []api.ServerPrompt{
		{Prompt: api.Prompt{Name: "override1"}},
		{Prompt: api.Prompt{Name: "override2"}},
	}

	result := mergePrompts(base, override)

	// Base prompts should come first (those not overridden)
	assert.Equal(t, "base1", result[0].Prompt.Name)
	assert.Equal(t, "base2", result[1].Prompt.Name)
	assert.Equal(t, "base3", result[2].Prompt.Name)

	// Then override prompts
	assert.Equal(t, "override1", result[3].Prompt.Name)
	assert.Equal(t, "override2", result[4].Prompt.Name)
}

func TestMergePromptsCompleteReplacement(t *testing.T) {
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

	result := mergePrompts(base, override)

	assert.Len(t, result, 1)
	assert.Equal(t, "test-prompt", result[0].Prompt.Name)
	assert.Equal(t, "Override description", result[0].Prompt.Description)
	assert.Len(t, result[0].Prompt.Arguments, 1)
	assert.Equal(t, "override_arg", result[0].Prompt.Arguments[0].Name)
	assert.False(t, result[0].Prompt.Arguments[0].Required)
}
