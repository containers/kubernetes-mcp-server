package kiali

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
)

type tracesOperations struct {
	singularName string
	tracesFunc   func(ctx context.Context, k *kialiclient.Kiali, namespace, name string, queryParams map[string]string) (string, error)
}

var tracesOpsMap = map[string]tracesOperations{
	"app": {
		singularName: "app",
		tracesFunc: func(ctx context.Context, k *kialiclient.Kiali, ns, name string, queryParams map[string]string) (string, error) {
			return k.AppTraces(ctx, ns, name, queryParams)
		},
	},
	"service": {
		singularName: "service",
		tracesFunc: func(ctx context.Context, k *kialiclient.Kiali, ns, name string, queryParams map[string]string) (string, error) {
			return k.ServiceTraces(ctx, ns, name, queryParams)
		},
	},
	"workload": {
		singularName: "workload",
		tracesFunc: func(ctx context.Context, k *kialiclient.Kiali, ns, name string, queryParams map[string]string) (string, error) {
			return k.WorkloadTraces(ctx, ns, name, queryParams)
		},
	},
}

func initGetTraces() []api.ServerTool {
	ret := make([]api.ServerTool, 0)

	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "kiali_get_traces",
			Description: "Gets traces for a specific resource (app, service, workload) in a namespace",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"resource_type": {
						Type:        "string",
						Description: "Type of resource to get metrics for (app, service, workload)",
						Enum:        []any{"app", "service", "workload"},
					},
					"namespace": {
						Type:        "string",
						Description: "Namespace to get resources from",
					},
					"resource_name": {
						Type:        "string",
						Description: "Name of the resource to get details for (optional string - if provided, gets details; if empty, lists all).",
					},
					"startMicros": {
						Type:        "string",
						Description: "Start time for traces in microseconds since epoch (optional)",
					},
					"endMicros": {
						Type:        "string",
						Description: "End time for traces in microseconds since epoch (optional)",
					},
					"limit": {
						Type:        "integer",
						Description: "Maximum number of traces to return (default: 100)",
						Minimum:     ptr.To(float64(1)),
					},
					"minDuration": {
						Type:        "integer",
						Description: "Minimum trace duration in microseconds (optional)",
						Minimum:     ptr.To(float64(0)),
					},
					"tags": {
						Type:        "string",
						Description: "JSON string of tags to filter traces (optional)",
					},
					"clusterName": {
						Type:        "string",
						Description: "Cluster name for multi-cluster environments (optional)",
					},
				},
				Required: []string{"resource_type", "namespace", "resource_name"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Get Traces for a Resource",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: TracesHandler,
	})

	return ret
}

func TracesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Extract parameters
	resourceType, _ := params.GetArguments()["resource_type"].(string)
	namespace, _ := params.GetArguments()["namespace"].(string)
	resourceName, _ := params.GetArguments()["resource_name"].(string)

	resourceType = strings.ToLower(strings.TrimSpace(resourceType))
	namespace = strings.TrimSpace(namespace)
	resourceName = strings.TrimSpace(resourceName)

	if resourceType == "" {
		return api.NewToolCallResult("", fmt.Errorf("resource_type is required")), nil
	}
	if namespace == "" || len(strings.Split(namespace, ",")) != 1 {
		return api.NewToolCallResult("", fmt.Errorf("namespace is required")), nil
	}
	if resourceName == "" {
		return api.NewToolCallResult("", fmt.Errorf("resource_name is required")), nil
	}

	k := params.NewKiali()

	ops, ok := tracesOpsMap[resourceType]
	if !ok {
		return api.NewToolCallResult("", fmt.Errorf("invalid resource type: %s", resourceType)), nil
	}

	queryParams := make(map[string]string)
	if startMicros, ok := params.GetArguments()["startMicros"].(string); ok && startMicros != "" {
		queryParams["startMicros"] = startMicros
	}
	if endMicros, ok := params.GetArguments()["endMicros"].(string); ok && endMicros != "" {
		queryParams["endMicros"] = endMicros
	}
	if limit, ok := params.GetArguments()["limit"].(string); ok && limit != "" {
		queryParams["limit"] = limit
	}
	if minDuration, ok := params.GetArguments()["minDuration"].(string); ok && minDuration != "" {
		queryParams["minDuration"] = minDuration
	}
	if tags, ok := params.GetArguments()["tags"].(string); ok && tags != "" {
		queryParams["tags"] = tags
	}
	if clusterName, ok := params.GetArguments()["clusterName"].(string); ok && clusterName != "" {
		queryParams["clusterName"] = clusterName
	}
	content, err := ops.tracesFunc(params.Context, k, namespace, resourceName, queryParams)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get %s traces: %v", ops.singularName, err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
