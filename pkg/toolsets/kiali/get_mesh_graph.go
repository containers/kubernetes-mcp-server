package kiali

import (
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func initGetMeshGraph() []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "kiali_get_mesh_graph",
			Description: "Returns the topology of a specific namespaces, health, status of the mesh and namespaces. Use this for high-level overviews",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespace": {
						Type:        "string",
						Description: "Optional single namespace to include in the graph (alternative to namespaces)",
					},
					"namespaces": {
						Type:        "string",
						Description: "Optional comma-separated list of namespaces to include in the graph",
					},
					"rateInterval": {
						Type:        "string",
						Description: "Rate interval for fetching (e.g., '10m', '5m', '1h'). Default: '60s'",
					},
					"graphType": {
						Type:        "string",
						Description: "Type of graph to return: 'versionedApp', 'app', 'service', 'workload', 'mesh'. Default: 'versionedApp'",
					},
				},
				Required: []string{},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Topology: Mesh, Graph, Health, and Status",
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

	// Parse arguments: allow either `namespace` or `namespaces` (comma-separated string)
	namespaces := make([]string, 0)
	if v, ok := params.GetArguments()["namespace"].(string); ok {
		v = strings.TrimSpace(v)
		if v != "" {
			namespaces = append(namespaces, v)
		}
	}
	if v, ok := params.GetArguments()["namespaces"].(string); ok {
		for _, ns := range strings.Split(v, ",") {
			ns = strings.TrimSpace(ns)
			if ns != "" {
				namespaces = append(namespaces, ns)
			}
		}
	}
	// Deduplicate namespaces if both provided
	if len(namespaces) > 1 {
		seen := map[string]struct{}{}
		unique := make([]string, 0, len(namespaces))
		for _, ns := range namespaces {
			key := strings.TrimSpace(ns)
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			unique = append(unique, key)
		}
		namespaces = unique
	}

	// Extract optional query parameters
	queryParams := make(map[string]string)
	if rateInterval, ok := params.GetArguments()["rateInterval"].(string); ok && rateInterval != "" {
		queryParams["rateInterval"] = rateInterval
	}
	if graphType, ok := params.GetArguments()["graphType"].(string); ok && graphType != "" {
		queryParams["graphType"] = graphType
	}
	k := params.NewKiali()
	content, err := k.GetMeshGraph(params.Context, namespaces, queryParams)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to retrieve mesh graph: %v", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
