package kiali

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func initMeshStatus() []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "mesh_status",
			Description: "Get the status of mesh components including Istio, Kiali, Grafana, Prometheus and their interactions, versions, and health status",
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: map[string]*jsonschema.Schema{},
				Required:   []string{},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Mesh Status: Components Overview",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: meshStatusHandler,
	})
	return ret
}

func meshStatusHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	content, err := params.MeshStatus(params.Context)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to retrieve mesh status: %v", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
