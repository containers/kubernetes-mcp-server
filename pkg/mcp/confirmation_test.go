package mcp

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config/configtest"
	"github.com/containers/kubernetes-mcp-server/pkg/confirmation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
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
	configtest.OverlayTOML(s.T(), &s.Cfg, `
[[confirmation_rules]]
tool = "pods_list"
message = "List pods?"
`)
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
	configtest.OverlayTOML(s.T(), &s.Cfg, `
[[confirmation_rules]]
tool = "pods_list"
message = "List pods?"
`)
	s.InitMcpClient(test.WithElicitationHandler(
		func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	))
	result, err := s.CallTool("pods_list", map[string]any{})
	s.Run("returns confirmation denied error", func() {
		s.NoError(err)
		s.Require().NotNil(result)
		s.True(result.IsError)
		s.Contains(result.Content[0].(*mcp.TextContent).Text, confirmation.ErrConfirmationDenied.Error())
	})
}

func (s *ConfirmationRulesSuite) TestToolRuleNoElicitationSupportFallbackDeny() {
	configtest.OverlayTOML(s.T(), &s.Cfg, `
confirmation_fallback = "deny"

[[confirmation_rules]]
tool = "pods_list"
message = "List pods?"
`)
	// No elicitation handler = client does not support elicitation
	s.InitMcpClient()
	result, err := s.CallTool("pods_list", map[string]any{})
	s.Run("blocked when fallback is deny", func() {
		s.NoError(err)
		s.Require().NotNil(result)
		s.True(result.IsError)
		s.Contains(result.Content[0].(*mcp.TextContent).Text, confirmation.ErrConfirmationDenied.Error())
	})
}

func (s *ConfirmationRulesSuite) TestToolRuleNoElicitationSupportFallbackAllow() {
	configtest.OverlayTOML(s.T(), &s.Cfg, `
confirmation_fallback = "allow"

[[confirmation_rules]]
tool = "pods_list"
message = "List pods?"
`)
	// No elicitation handler = client does not support elicitation
	s.InitMcpClient()
	result, err := s.CallTool("pods_list", map[string]any{})
	s.Run("proceeds when fallback is allow", func() {
		s.NoError(err)
		s.Require().NotNil(result)
		s.False(result.IsError)
	})
}

func (s *ConfirmationRulesSuite) TestDestructiveRuleMatchesDestructiveTools() {
	configtest.OverlayTOML(s.T(), &s.Cfg, `
[[confirmation_rules]]
destructive = true
message = "Destructive operation."
`)
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
	configtest.OverlayTOML(s.T(), &s.Cfg, `
[[confirmation_rules]]
tool = "pods_list"
message = "Listing pods."

[[confirmation_rules]]
tool = "pods_list"
message = "Are you sure?"
`)
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

func (s *ConfirmationRulesSuite) TestToolRuleDoesNotMatchOtherTools() {
	configtest.OverlayTOML(s.T(), &s.Cfg, `
confirmation_fallback = "deny"

[[confirmation_rules]]
tool = "namespaces_list"
message = "Listing namespaces."
`)
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
