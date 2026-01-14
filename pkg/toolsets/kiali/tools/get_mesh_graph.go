package tools

import (
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/internal/defaults"
)

func InitGetMeshGraph() []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	name := defaults.ToolsetName() + "_mesh_graph"
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        name,
			Description: "Returns the topology of a specific namespaces, health, status of the mesh and namespaces. Includes a mesh health summary overview with aggregated counts of healthy, degraded, and failing apps, workloads, and services. Use this for high-level overviews",
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
						Description: "Optional rate interval for fetching (e.g., '10m', '5m', '1h').",
						Default:     api.ToRawMessage(kialiclient.DefaultRateInterval),
					},
					"graphType": {
						Type:        "string",
						Description: "Optional type of graph to return: 'versionedApp', 'app', 'service', 'workload', 'mesh'",
						Default:     api.ToRawMessage(kialiclient.DefaultGraphType),
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
	if err := setQueryParam(params, queryParams, "rateInterval", kialiclient.DefaultRateInterval); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	if err := setQueryParam(params, queryParams, "graphType", kialiclient.DefaultGraphType); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	kiali := kialiclient.NewKiali(params, params.RESTConfig())
	content, err := kiali.GetMeshGraph(params.Context, namespaces, queryParams)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to retrieve mesh graph: %w", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
