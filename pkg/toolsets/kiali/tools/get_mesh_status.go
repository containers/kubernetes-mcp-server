package tools

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/internal/defaults"
)

func InitGetMeshStatus() []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	name := defaults.ToolsetName() + "_get_mesh_status"
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        name,
			Description: "Retrieves the high-level health, topology, and environment details of the Istio service mesh. Returns multi-cluster control plane status (istiod), data plane namespace health (including ambient mesh status), observability stack health (Prometheus, Grafana...), and component connectivity. Use this tool as the first step to diagnose mesh-wide issues, verify Istio/Kiali versions, or check overall health before drilling into specific workloads.",
			InputSchema: &jsonschema.Schema{
				Type:     "object",
				Required: []string{},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Get Mesh Status",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: getMeshStatusHandler,
	})
	return ret
}

func getMeshStatusHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	kiali := kialiclient.NewKiali(params, params.RESTConfig())
	content, err := kiali.ExecuteRequest(params.Context, KialiGetMeshStatusEndpoint, nil)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to retrieve mesh status: %w", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
