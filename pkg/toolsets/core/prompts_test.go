package core

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitPrompts(t *testing.T) {
	prompts := initPrompts()

	require.NotNil(t, prompts, "prompts should not be nil")
	assert.Greater(t, len(prompts), 0, "should have at least one prompt")

	promptNames := make(map[string]bool)
	for _, p := range prompts {
		assert.NotEmpty(t, p.Prompt.Name, "prompt name should not be empty")
		assert.NotEmpty(t, p.Prompt.Description, "prompt description should not be empty")
		assert.NotNil(t, p.Handler, "prompt handler should not be nil")

		promptNames[p.Prompt.Name] = true
	}

	expectedPrompts := []string{
		"troubleshoot-pod",
		"deploy-application",
		"scale-deployment",
		"investigate-cluster-health",
		"debug-networking",
		"review-resource-usage",
	}

	for _, expected := range expectedPrompts {
		assert.True(t, promptNames[expected], "should contain prompt: %s", expected)
	}
}

func TestCoreToolset_GetPrompts(t *testing.T) {
	toolset := &Toolset{}
	prompts := toolset.GetPrompts(nil)

	require.NotNil(t, prompts)
	assert.Greater(t, len(prompts), 0)

	for _, p := range prompts {
		assert.NotEmpty(t, p.Prompt.Name)
		assert.NotNil(t, p.Handler)
	}
}

func TestPromptArgumentDefinitions(t *testing.T) {
	prompts := initPrompts()
	require.NotNil(t, prompts)

	tests := []struct {
		promptName   string
		expectedArgs int
		requiredArgs []string
		optionalArgs []string
	}{
		{
			promptName:   "troubleshoot-pod",
			expectedArgs: 2,
			requiredArgs: []string{"namespace", "pod_name"},
		},
		{
			promptName:   "deploy-application",
			expectedArgs: 2,
			requiredArgs: []string{"app_name"},
			optionalArgs: []string{"namespace"},
		},
		{
			promptName:   "scale-deployment",
			expectedArgs: 3,
			requiredArgs: []string{"deployment_name", "namespace", "replicas"},
		},
		{
			promptName:   "investigate-cluster-health",
			expectedArgs: 0,
		},
		{
			promptName:   "debug-networking",
			expectedArgs: 3,
			optionalArgs: []string{"source_pod", "source_namespace", "target_service"},
		},
		{
			promptName:   "review-resource-usage",
			expectedArgs: 1,
			optionalArgs: []string{"namespace"},
		},
	}

	promptMap := make(map[string]*api.ServerPrompt)
	for i := range prompts {
		promptMap[prompts[i].Prompt.Name] = &prompts[i]
	}

	for _, tt := range tests {
		t.Run(tt.promptName, func(t *testing.T) {
			prompt, exists := promptMap[tt.promptName]
			require.True(t, exists, "prompt %s should exist", tt.promptName)

			assert.Len(t, prompt.Prompt.Arguments, tt.expectedArgs)

			argMap := make(map[string]bool)
			requiredMap := make(map[string]bool)

			for _, arg := range prompt.Prompt.Arguments {
				argMap[arg.Name] = true
				if arg.Required {
					requiredMap[arg.Name] = true
				}
			}

			for _, reqArg := range tt.requiredArgs {
				assert.True(t, argMap[reqArg], "should have argument: %s", reqArg)
				assert.True(t, requiredMap[reqArg], "argument should be required: %s", reqArg)
			}

			for _, optArg := range tt.optionalArgs {
				assert.True(t, argMap[optArg], "should have argument: %s", optArg)
				assert.False(t, requiredMap[optArg], "argument should be optional: %s", optArg)
			}
		})
	}
}
