package kubevirt

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
)

// mockPromptCallRequest implements api.PromptCallRequest for testing
type mockPromptCallRequest struct {
	args map[string]string
}

func (m *mockPromptCallRequest) GetArguments() map[string]string {
	return m.args
}

type VMTroubleshootSuite struct {
	suite.Suite
}

func (s *VMTroubleshootSuite) TestVMTroubleshootPrompt() {
	s.Run("prompt is registered", func() {
		prompts := initVMTroubleshoot()
		s.Require().Len(prompts, 1, "Expected 1 prompt")
		s.Equal("vm-troubleshoot", prompts[0].Prompt.Name)
		s.Equal("VirtualMachine Troubleshoot", prompts[0].Prompt.Title)
		s.Len(prompts[0].Prompt.Arguments, 2, "Expected 2 arguments")
	})

	s.Run("generates troubleshooting guide with valid arguments", func() {
		prompts := initVMTroubleshoot()
		handler := prompts[0].Handler

		params := api.PromptHandlerParams{
			PromptCallRequest: &mockPromptCallRequest{
				args: map[string]string{
					"namespace": "test-ns",
					"name":      "test-vm",
				},
			},
		}

		result, err := handler(params)
		s.Require().NoError(err)
		s.Require().NotNil(result)
		s.Require().Len(result.Messages, 2, "Expected 2 messages")

		content := result.Messages[0].Content.Text
		s.Contains(content, "# VirtualMachine Troubleshooting Guide")
		s.Contains(content, "test-vm")
		s.Contains(content, "test-ns")
		s.Contains(content, "Step 1: Check VirtualMachine Status")
		s.Contains(content, "resources_get")
		s.Contains(content, "VirtualMachineInstance")
		s.Contains(content, "virt-launcher")
	})

	s.Run("returns error for missing namespace", func() {
		prompts := initVMTroubleshoot()
		handler := prompts[0].Handler

		params := api.PromptHandlerParams{
			PromptCallRequest: &mockPromptCallRequest{
				args: map[string]string{
					"name": "test-vm",
				},
			},
		}

		result, err := handler(params)
		s.Error(err)
		s.Nil(result)
		s.Contains(err.Error(), "namespace")
	})

	s.Run("returns error for missing name", func() {
		prompts := initVMTroubleshoot()
		handler := prompts[0].Handler

		params := api.PromptHandlerParams{
			PromptCallRequest: &mockPromptCallRequest{
				args: map[string]string{
					"namespace": "test-ns",
				},
			},
		}

		result, err := handler(params)
		s.Error(err)
		s.Nil(result)
		s.Contains(err.Error(), "name")
	})
}

func TestVMTroubleshoot(t *testing.T) {
	suite.Run(t, new(VMTroubleshootSuite))
}
