package tools

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/internal/defaults"
)

func InitGetMeshTrafficGraph() []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	name := defaults.ToolsetName() + "_get_mesh_traffic_graph"
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        name,
			Description: "Returns service-to-service traffic topology, dependencies, and network metrics (throughput, response time, mTLS) for the specified namespaces. Use this to diagnose routing issues, latency, or find upstream/downstream dependencies.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespaces": {
						Type:        "string",
						Description: "Comma-separated list of namespaces to map",
					},
					"graphType": {
						Type:        "string",
						Description: "Granularity of the graph. 'app' aggregates by app name, 'versionedApp' separates by versions, 'workload' maps specific pods/deployments. Default: versionedApp.",
						Default:     api.ToRawMessage(DefaultGraphType),
						Enum:        []any{"app", "versionedApp", "service", "workload"},
					},
					"clusterName": {
						Type:        "string",
						Description: "Optional cluster name to include in the graph. Default is the cluster name in the Kiali configuration (KubeConfig).",
					},
				},
				Required: []string{"namespaces"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Get Mesh Traffic Graph",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: getMeshGraphHandler,
	})
	return ret
}

func getMeshGraphHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	kiali := kialiclient.NewKiali(params, params.RESTConfig())
	arguments := params.GetArguments()
	content, err := kiali.ExecuteRequest(params.Context, KialiGetMeshTrafficGraphEndpoint, arguments)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to retrieve mesh traffic graph: %w", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
