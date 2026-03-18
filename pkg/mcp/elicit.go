package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/mcplog"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type sessionElicitor struct{}

var _ api.Elicitor = &sessionElicitor{}

func (s *sessionElicitor) Elicit(ctx context.Context, params *api.ElicitParams) (*api.ElicitResult, error) {
	session, ok := ctx.Value(mcplog.MCPSessionContextKey).(*mcp.ServerSession)
	if !ok || session == nil {
		return nil, fmt.Errorf("no MCP session found in context")
	}

	result, err := session.Elicit(ctx, &mcp.ElicitParams{
		Message:         params.Message,
		RequestedSchema: params.RequestedSchema,
		URL:             params.URL,
		ElicitationID:   params.ElicitationID,
	})
	if err != nil {
		// The go-sdk does not export a typed error for unsupported elicitation.
		// This string check mirrors the go-sdk's own test approach (mcp/mcp_test.go).
		// The go-sdk returns three variants: "client does not support elicitation",
		// "client does not support "form" elicitation", and "client does not support "url" elicitation".
		if strings.Contains(err.Error(), "does not support") && strings.Contains(err.Error(), "elicitation") {
			return nil, fmt.Errorf("%w: %s", api.ErrElicitationNotSupported, err.Error())
		}
		return nil, err
	}

	return &api.ElicitResult{Action: result.Action, Content: result.Content}, nil
}
