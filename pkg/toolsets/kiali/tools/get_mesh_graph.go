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

// REGEX_RATE_INTERVAL_VALID_TYPES: integer followed by s, m, h, or d (e.g. 10m, 5m, 1h, 30s, 7d).
const REGEX_RATE_INTERVAL_VALID_TYPES = `^\d+[smhd]$`

func InitGetMeshGraph() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        defaults.ToolsetName() + "_mesh_status",
				Description: "Returns the status of the mesh. Includes a mesh health summary overview with aggregated counts of healthy, degraded, and failing apps, workloads, and services. Use this for high-level overviews",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespaces": {
							Type:        "string",
							Description: "Optional comma-separated list of namespaces. If empty, will return the mesh status for all namespaces.",
						},
						"rateInterval": {
							Type:        "string",
							Description: "Optional rate interval for fetching (e.g., '10m', '5m', '1h').",
							Default:     api.ToRawMessage(kialiclient.DefaultRateInterval),
							Pattern:     REGEX_RATE_INTERVAL_VALID_TYPES,
						},
						"type": {
							Type:        "string",
							Description: "Optional type health focused in : 'app', 'service', 'workload'",
							Default:     api.ToRawMessage(kialiclient.DefaultHealthType),
							Enum:        []any{"app", "service", "workload"},
						},
					},
					Required: []string{},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Mesh Status (" + defaults.ToolsetName() + ")",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(false),
					OpenWorldHint:   ptr.To(true),
				},
			}, Handler: getMeshStatusHandler,
		},
		{
			Tool: api.Tool{
				Name:        defaults.ToolsetName() + "_topology_graph",
				Description: "Returns the topology of a specific namespaces, health, status of the mesh and namespaces. Includes a mesh health summary overview with aggregated counts of healthy, degraded, and failing apps, workloads, and services. Use this for high-level overviews",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespaces": {
							Type:        "string",
							Description: "Comma-separated list of namespaces to include in the graph",
						},
						"rateInterval": {
							Type:        "string",
							Description: "Optional rate interval for fetching (e.g., '10m', '5m', '1h').",
							Default:     api.ToRawMessage(kialiclient.DefaultRateInterval),
							Pattern:     REGEX_RATE_INTERVAL_VALID_TYPES,
						},
						"graphType": {
							Type:        "string",
							Description: "Optional type of graph to return: 'versionedApp', 'app', 'service', 'workload', 'mesh'",
							Default:     api.ToRawMessage(kialiclient.DefaultGraphType),
							Enum:        []any{"versionedApp", "app", "service", "workload", "mesh"},
						},
					},
					Required: []string{"namespaces"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Topology Graph (" + defaults.ToolsetName() + ")",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(false),
					OpenWorldHint:   ptr.To(true),
				},
			}, Handler: getMeshGraphHandler,
		},
	}
}

func getMeshStatusHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	namespaces := cleanNamespaces(params)

	queryParams := make(map[string]string)
	if err := setQueryParam(params, queryParams, "rateInterval", kialiclient.DefaultRateInterval); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	if err := setQueryParam(params, queryParams, "type", kialiclient.DefaultHealthType); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	kiali := kialiclient.NewKiali(params, params.RESTConfig())
	content, err := kiali.GetMeshStatus(params.Context, namespaces, queryParams)

	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to retrieve mesh status: %w", err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}

func cleanNamespaces(params api.ToolHandlerParams) []string {
	namespaces := make([]string, 0)
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

	return namespaces
}

func getMeshGraphHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Parse arguments: allow either `namespace` or `namespaces` (comma-separated string)
	namespaces := cleanNamespaces(params)

	if len(namespaces) == 0 {
		return api.NewToolCallResult("", fmt.Errorf("no namespaces provided")), nil
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
