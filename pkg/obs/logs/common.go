package logs

import (
	"context"
	"encoding/json"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/client-go/dynamic"

	"github.com/rhobs/obs-mcp/pkg/logs/loki"
)

// ToolParams is a subset of api.ToolHandlerParams and contains only fields required by logs tool handlers.
type ToolParams struct {
	context       context.Context
	arguments     map[string]any
	dynamicClient dynamic.Interface
	config        *Config
	newLokiLoader func(url, tenant string) (loki.Loader, error)
}

// ToolHandler is the signature shared by all logs tool handlers.
type ToolHandler[T any] func(params ToolParams) (T, error)

func ToMCPHandler[T any](
	newLokiLoader func(ctx context.Context, url, tenant string) (loki.Loader, error),
	dynamicClient dynamic.Interface,
	config *Config,
	handler ToolHandler[T],
) mcp.ToolHandlerFor[map[string]any, T] {
	return func(ctx context.Context, request *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, T, error) {
		output, err := handler(ToolParams{
			context:       ctx,
			arguments:     args,
			dynamicClient: dynamicClient,
			config:        config,
			newLokiLoader: func(url, tenant string) (loki.Loader, error) {
				return newLokiLoader(ctx, url, tenant)
			},
		})
		return nil, output, err
	}
}

func ToServerHandler[T any](
	newLokiLoader func(params api.ToolHandlerParams, url, tenant string) (loki.Loader, error),
	handler ToolHandler[T],
) api.ToolHandlerFunc {
	return func(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
		config := GetConfig(params)
		output, err := handler(ToolParams{
			context:       params.Context,
			arguments:     params.GetArguments(),
			dynamicClient: params.DynamicClient(),
			config:        config,
			newLokiLoader: func(url, tenant string) (loki.Loader, error) {
				return newLokiLoader(params, url, tenant)
			},
		})
		if err != nil {
			return api.NewToolCallResult("", err), nil
		}

		jsonBytes, err := json.Marshal(output)
		if err != nil {
			return nil, err
		}
		return api.NewToolCallResult(string(jsonBytes), nil), nil
	}
}
