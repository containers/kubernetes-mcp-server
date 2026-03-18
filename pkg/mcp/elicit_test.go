package mcp

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
	"k8s.io/utils/ptr"
)

type ElicitationSuite struct {
	BaseMcpSuite
	originalToolsets []api.Toolset
}

func (s *ElicitationSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.originalToolsets = toolsets.Toolsets()
}

func (s *ElicitationSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	toolsets.Clear()
	for _, toolset := range s.originalToolsets {
		toolsets.Register(toolset)
	}
}

func (s *ElicitationSuite) registerElicitingToolset(handler api.ToolHandlerFunc) {
	testToolset := &mockElicitToolset{
		tools: []api.ServerTool{
			{
				Tool: api.Tool{
					Name:        "elicit_test_tool",
					Description: "Tool that uses elicitation for testing",
					Annotations: api.ToolAnnotations{
						ReadOnlyHint: ptr.To(true),
					},
					InputSchema: &jsonschema.Schema{
						Type:       "object",
						Properties: make(map[string]*jsonschema.Schema),
					},
				},
				Handler: handler,
			},
		},
	}

	toolsets.Clear()
	toolsets.Register(testToolset)
	s.Cfg.Toolsets = []string{"elicit-test"}
}

func (s *ElicitationSuite) TestElicitationAccepted() {
	s.registerElicitingToolset(func(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
		result, err := params.Elicit(params.Context, &api.ElicitParams{Message: "Please confirm"})
		if err != nil {
			return nil, err
		}
		return api.NewToolCallResult("action="+result.Action+",name="+result.Content["name"].(string), nil), nil
	})

	s.InitMcpClient(test.WithElicitationHandler(
		func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{
				Action:  "accept",
				Content: map[string]any{"name": "test-value"},
			}, nil
		},
	))

	toolResult, err := s.CallTool("elicit_test_tool", map[string]any{})

	s.Run("returns accepted elicitation content", func() {
		s.NoError(err)
		s.Require().NotNil(toolResult)
		s.False(toolResult.IsError)
		s.Require().Len(toolResult.Content, 1)
		textContent, ok := toolResult.Content[0].(*mcp.TextContent)
		s.Require().True(ok)
		s.Equal("action=accept,name=test-value", textContent.Text)
	})
}

func (s *ElicitationSuite) TestElicitationDeclined() {
	s.registerElicitingToolset(func(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
		result, err := params.Elicit(params.Context, &api.ElicitParams{Message: "Please confirm"})
		if err != nil {
			return nil, err
		}
		return api.NewToolCallResult("action="+result.Action, nil), nil
	})

	s.InitMcpClient(test.WithElicitationHandler(
		func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	))

	toolResult, err := s.CallTool("elicit_test_tool", map[string]any{})

	s.Run("returns declined action", func() {
		s.NoError(err)
		s.Require().NotNil(toolResult)
		s.False(toolResult.IsError)
		s.Require().Len(toolResult.Content, 1)
		textContent, ok := toolResult.Content[0].(*mcp.TextContent)
		s.Require().True(ok)
		s.Equal("action=decline", textContent.Text)
	})
}

func (s *ElicitationSuite) TestElicitationWithUnsupportedClient() {
	s.registerElicitingToolset(func(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
		_, err := params.Elicit(params.Context, &api.ElicitParams{Message: "Please confirm"})
		if err != nil {
			if errors.Is(err, api.ErrElicitationNotSupported) {
				return api.NewToolCallResult("fallback-result", nil), nil
			}
			return nil, err
		}
		return api.NewToolCallResult("should-not-reach", nil), nil
	})

	// No ElicitationHandler = client does not support elicitation
	s.InitMcpClient()

	toolResult, err := s.CallTool("elicit_test_tool", map[string]any{})

	s.Run("tool handles unsupported elicitation gracefully", func() {
		s.NoError(err)
		s.Require().NotNil(toolResult)
		s.False(toolResult.IsError)
		s.Require().Len(toolResult.Content, 1)
		textContent, ok := toolResult.Content[0].(*mcp.TextContent)
		s.Require().True(ok)
		s.Equal("fallback-result", textContent.Text)
	})
}

func (s *ElicitationSuite) TestElicitationURLAccepted() {
	s.registerElicitingToolset(func(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
		result, err := params.Elicit(params.Context, &api.ElicitParams{
			Message: "Please complete the form",
			URL:     "https://example.com/form",
		})
		if err != nil {
			return nil, err
		}
		return api.NewToolCallResult("action="+result.Action, nil), nil
	})

	s.InitMcpClient(
		test.WithElicitationHandler(
			func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				return &mcp.ElicitResult{Action: "accept"}, nil
			},
		),
		test.WithClientCapabilities(&mcp.ClientCapabilities{
			Elicitation: &mcp.ElicitationCapabilities{
				URL: &mcp.URLElicitationCapabilities{},
			},
		}),
	)

	toolResult, err := s.CallTool("elicit_test_tool", map[string]any{})

	s.Run("returns accepted action for URL elicitation", func() {
		s.NoError(err)
		s.Require().NotNil(toolResult)
		s.False(toolResult.IsError)
		s.Require().Len(toolResult.Content, 1)
		textContent, ok := toolResult.Content[0].(*mcp.TextContent)
		s.Require().True(ok)
		s.Equal("action=accept", textContent.Text)
	})
}

func (s *ElicitationSuite) TestElicitationURLWithElicitationID() {
	s.registerElicitingToolset(func(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
		result, err := params.Elicit(params.Context, &api.ElicitParams{
			Message:       "Please complete the form",
			URL:           "https://example.com/form",
			ElicitationID: "elicit-123",
		})
		if err != nil {
			return nil, err
		}
		return api.NewToolCallResult("action="+result.Action, nil), nil
	})

	s.InitMcpClient(
		test.WithElicitationHandler(
			func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				if req.Params.ElicitationID != "elicit-123" {
					return nil, fmt.Errorf("expected elicitationID 'elicit-123', got '%s'", req.Params.ElicitationID)
				}
				return &mcp.ElicitResult{Action: "accept"}, nil
			},
		),
		test.WithClientCapabilities(&mcp.ClientCapabilities{
			Elicitation: &mcp.ElicitationCapabilities{
				URL: &mcp.URLElicitationCapabilities{},
			},
		}),
	)

	toolResult, err := s.CallTool("elicit_test_tool", map[string]any{})

	s.Run("forwards elicitationID to the client", func() {
		s.NoError(err)
		s.Require().NotNil(toolResult)
		s.False(toolResult.IsError)
		s.Require().Len(toolResult.Content, 1)
		textContent, ok := toolResult.Content[0].(*mcp.TextContent)
		s.Require().True(ok)
		s.Equal("action=accept", textContent.Text)
	})
}

func (s *ElicitationSuite) TestElicitationURLWithUnsupportedClient() {
	s.registerElicitingToolset(func(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
		_, err := params.Elicit(params.Context, &api.ElicitParams{
			Message: "Please complete the form",
			URL:     "https://example.com/form",
		})
		if err != nil {
			if errors.Is(err, api.ErrElicitationNotSupported) {
				return api.NewToolCallResult("url-fallback-result", nil), nil
			}
			return nil, err
		}
		return api.NewToolCallResult("should-not-reach", nil), nil
	})

	// Client supports elicitation but not URL mode (default capabilities = form only)
	s.InitMcpClient(test.WithElicitationHandler(
		func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "accept"}, nil
		},
	))

	toolResult, err := s.CallTool("elicit_test_tool", map[string]any{})

	s.Run("tool handles unsupported URL elicitation gracefully", func() {
		s.NoError(err)
		s.Require().NotNil(toolResult)
		s.False(toolResult.IsError)
		s.Require().Len(toolResult.Content, 1)
		textContent, ok := toolResult.Content[0].(*mcp.TextContent)
		s.Require().True(ok)
		s.Equal("url-fallback-result", textContent.Text)
	})
}

// mockElicitToolset is a test toolset that provides tools using the Elicitor interface
type mockElicitToolset struct {
	tools []api.ServerTool
}

func (m *mockElicitToolset) GetName() string {
	return "elicit-test"
}

func (m *mockElicitToolset) GetDescription() string {
	return "Test toolset for elicitation"
}

func (m *mockElicitToolset) GetTools(_ api.Openshift) []api.ServerTool {
	return m.tools
}

func (m *mockElicitToolset) GetPrompts() []api.ServerPrompt {
	return nil
}

func TestElicitationSuite(t *testing.T) {
	suite.Run(t, new(ElicitationSuite))
}
