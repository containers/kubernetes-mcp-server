package tools

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/internal/defaults"
)

func InitListMeshClusters() []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	name := defaults.ToolsetName() + "_list_mesh_clusters"
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        name,
			Description: "Returns the list of Istio mesh clusters that Kiali can access. Each cluster includes its name and whether it is the home cluster (where Kiali is deployed). Use this tool to discover available mesh cluster names before calling other tools that accept a meshCluster parameter.",
			InputSchema: &jsonschema.Schema{
				Type:     "object",
				Required: []string{},
			},
			Annotations: api.ToolAnnotations{
				Title:           "List Mesh Clusters",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: listMeshClustersHandler,
	})
	return ret
}

func listMeshClustersHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	kiali := kialiclient.NewKiali(params, params.RESTConfig())
	content, err := kiali.ExecuteRequest(params.Context, KialiListClustersEndpoint, nil)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list mesh clusters: %w", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
