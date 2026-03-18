package mcp

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
	"k8s.io/utils/ptr"
)

type ConfirmationRulesSuite struct {
	BaseMcpSuite
}

func (s *ConfirmationRulesSuite) TestNoRulesConfigured() {
	s.InitMcpClient()
	result, err := s.CallTool("pods_list", map[string]any{})
	s.Run("tool executes normally", func() {
		s.NoError(err)
		s.Require().NotNil(result)
		s.False(result.IsError)
	})
}

func (s *ConfirmationRulesSuite) TestToolRuleMatchUserAccepts() {
	s.Cfg.ConfirmationRules = []api.ConfirmationRule{
		{Tool: "pods_list", Message: "List pods?"},
	}
	s.InitMcpClient(test.WithElicitationHandler(
		func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "accept"}, nil
		},
	))
	result, err := s.CallTool("pods_list", map[string]any{})
	s.Run("tool executes after acceptance", func() {
		s.NoError(err)
		s.Require().NotNil(result)
		s.False(result.IsError)
	})
}

func (s *ConfirmationRulesSuite) TestToolRuleMatchUserDeclines() {
	s.Cfg.ConfirmationRules = []api.ConfirmationRule{
		{Tool: "pods_list", Message: "List pods?"},
	}
	s.InitMcpClient(test.WithElicitationHandler(
		func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	))
	result, err := s.CallTool("pods_list", map[string]any{})
	s.Run("returns error content", func() {
		s.NoError(err)
		s.Require().NotNil(result)
		s.True(result.IsError)
	})
}

func (s *ConfirmationRulesSuite) TestToolRuleNoElicitationSupportFallbackDeny() {
	s.Cfg.ConfirmationFallback = "deny"
	s.Cfg.ConfirmationRules = []api.ConfirmationRule{
		{Tool: "pods_list", Message: "List pods?"},
	}
	// No elicitation handler = client does not support elicitation
	s.InitMcpClient()
	result, err := s.CallTool("pods_list", map[string]any{})
	s.Run("blocked when fallback is deny", func() {
		s.NoError(err)
		s.Require().NotNil(result)
		s.True(result.IsError)
	})
}

func (s *ConfirmationRulesSuite) TestToolRuleNoElicitationSupportFallbackAllow() {
	s.Cfg.ConfirmationFallback = "allow"
	s.Cfg.ConfirmationRules = []api.ConfirmationRule{
		{Tool: "pods_list", Message: "List pods?"},
	}
	// No elicitation handler = client does not support elicitation
	s.InitMcpClient()
	result, err := s.CallTool("pods_list", map[string]any{})
	s.Run("proceeds when fallback is allow", func() {
		s.NoError(err)
		s.Require().NotNil(result)
		s.False(result.IsError)
	})
}

func (s *ConfirmationRulesSuite) TestToolRuleWithInputMatch() {
	s.Cfg.ConfirmationFallback = "deny"
	s.Cfg.ConfirmationRules = []api.ConfirmationRule{
		{Tool: "pods_list", Input: map[string]any{"namespace": "kube-system"}, Message: "Listing kube-system pods."},
	}
	// No elicitation handler = client does not support elicitation
	s.InitMcpClient()

	s.Run("triggers on matching args", func() {
		result, err := s.CallTool("pods_list", map[string]any{"namespace": "kube-system"})
		s.NoError(err)
		s.Require().NotNil(result)
		s.True(result.IsError)
	})
	s.Run("does not trigger on non-matching args", func() {
		result, err := s.CallTool("pods_list", map[string]any{"namespace": "default"})
		s.NoError(err)
		s.Require().NotNil(result)
		s.False(result.IsError)
	})
}

func (s *ConfirmationRulesSuite) TestDestructiveRuleMatchesDestructiveTools() {
	s.Cfg.ConfirmationRules = []api.ConfirmationRule{
		{Destructive: ptr.To(true), Message: "Destructive operation."},
	}
	s.InitMcpClient(test.WithElicitationHandler(
		func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "accept"}, nil
		},
	))

	s.Run("non-destructive tool not affected", func() {
		result, err := s.CallTool("pods_list", map[string]any{})
		s.NoError(err)
		s.Require().NotNil(result)
		s.False(result.IsError)
	})
}

func (s *ConfirmationRulesSuite) TestMultipleToolRulesMatchMergedPrompt() {
	s.Cfg.ConfirmationRules = []api.ConfirmationRule{
		{Tool: "pods_list", Message: "Listing pods."},
		{Tool: "pods_list", Message: "Are you sure?"},
	}
	var receivedMessage string
	s.InitMcpClient(test.WithElicitationHandler(
		func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			receivedMessage = req.Params.Message
			return &mcp.ElicitResult{Action: "accept"}, nil
		},
	))
	result, err := s.CallTool("pods_list", map[string]any{})
	s.Run("single prompt with merged messages", func() {
		s.NoError(err)
		s.Require().NotNil(result)
		s.False(result.IsError)
		s.Contains(receivedMessage, "Listing pods.")
		s.Contains(receivedMessage, "Are you sure?")
	})
}

func (s *ConfirmationRulesSuite) TestMultipleRulesMixedFallbacksDenyWins() {
	s.Cfg.ConfirmationFallback = "allow"
	s.Cfg.ConfirmationRules = []api.ConfirmationRule{
		{Tool: "pods_list", Message: "msg1", Fallback: "allow"},
		{Tool: "pods_list", Message: "msg2", Fallback: "deny"},
	}
	// No elicitation handler = client does not support elicitation
	s.InitMcpClient()
	result, err := s.CallTool("pods_list", map[string]any{})
	s.Run("most restrictive fallback wins", func() {
		s.NoError(err)
		s.Require().NotNil(result)
		s.True(result.IsError)
	})
}

func (s *ConfirmationRulesSuite) TestToolRuleDoesNotMatchOtherTools() {
	s.Cfg.ConfirmationFallback = "deny"
	s.Cfg.ConfirmationRules = []api.ConfirmationRule{
		{Tool: "namespaces_list", Message: "Listing namespaces."},
	}
	// No elicitation handler = client does not support elicitation
	s.InitMcpClient()
	result, err := s.CallTool("pods_list", map[string]any{})
	s.Run("unmatched tool executes normally", func() {
		s.NoError(err)
		s.Require().NotNil(result)
		s.False(result.IsError)
	})
}

func TestConfirmationRules(t *testing.T) {
	suite.Run(t, new(ConfirmationRulesSuite))
}
