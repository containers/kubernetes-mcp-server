package kiali

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func initNamespaces() []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "namespaces",
			Description: "Get all namespaces in the mesh that the user has access to",
			InputSchema: &jsonschema.Schema{
				Type: "object",
			},
			Annotations: api.ToolAnnotations{
				Title:           "Namespaces: List",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: namespacesHandler,
	})
	return ret
}

func namespacesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	k := params.NewKiali()
	content, err := k.ListNamespaces(params.Context)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list namespaces: %v", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
