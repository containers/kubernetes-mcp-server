package tools

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/internal/defaults"
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

func InitGetTraces() []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	name := defaults.ToolsetName() + "_get_traces"
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        name,
			Description: "Gets traces for a specific resource (app, service, workload) in a namespace, or gets detailed information for a specific trace by its ID. If traceId is provided, it returns detailed trace information and other parameters are not required.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"traceId": {
						Type:        "string",
						Description: "Unique identifier of the trace to retrieve detailed information for. If provided, this will return detailed trace information and other parameters (resource_type, namespace, resource_name) are not required.",
					},
					"resource_type": {
						Type:        "string",
						Description: "Type of resource to get traces for (app, service, workload). Required if traceId is not provided.",
						Enum:        []any{"app", "service", "workload"},
					},
					"namespace": {
						Type:        "string",
						Description: "Namespace to get resources from. Required if traceId is not provided.",
					},
					"resource_name": {
						Type:        "string",
						Description: "Name of the resource to get traces for. Required if traceId is not provided.",
					},
					"startMicros": {
						Type:        "string",
						Description: "Start time for traces in microseconds since epoch (optional, defaults to 10 minutes before current time if not provided, only used when traceId is not provided)",
					},
					"endMicros": {
						Type:        "string",
						Description: "End time for traces in microseconds since epoch (optional, defaults to 10 minutes after startMicros if not provided, only used when traceId is not provided)",
					},
					"limit": {
						Type:        "integer",
						Description: "Maximum number of traces to return (default: 100, only used when traceId is not provided)",
						Minimum:     ptr.To(float64(1)),
						Default:     api.ToRawMessage(kialiclient.DefaultLimit),
					},
					"minDuration": {
						Type:        "integer",
						Description: "Minimum trace duration in microseconds (optional, only used when traceId is not provided)",
						Minimum:     ptr.To(float64(0)),
					},
					"tags": {
						Type:        "string",
						Description: "JSON string of tags to filter traces (optional, only used when traceId is not provided)",
					},
					"clusterName": {
						Type:        "string",
						Description: "Cluster name for multi-cluster environments (optional, only used when traceId is not provided)",
					},
				},
				Required: []string{},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Get Traces for a Resource or Trace Details",
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
	kiali := kialiclient.NewKiali(params, params.RESTConfig())

	// Check if traceId is provided - if so, get trace details directly
	if traceIdVal, ok := params.GetArguments()["traceId"].(string); ok && traceIdVal != "" {
		traceId := strings.TrimSpace(traceIdVal)
		content, err := kiali.TraceDetails(params.Context, traceId)
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("failed to get trace details: %w", err)), nil
		}
		return api.NewToolCallResult(content, nil), nil
	}

	// Otherwise, get traces for a resource (existing functionality)
	resourceType, _ := params.GetArguments()["resource_type"].(string)
	namespace, _ := params.GetArguments()["namespace"].(string)
	resourceName, _ := params.GetArguments()["resource_name"].(string)

	resourceType = strings.ToLower(strings.TrimSpace(resourceType))
	namespace = strings.TrimSpace(namespace)
	resourceName = strings.TrimSpace(resourceName)

	if resourceType == "" {
		return api.NewToolCallResult("", fmt.Errorf("resource_type is required when traceId is not provided")), nil
	}
	if namespace == "" || len(strings.Split(namespace, ",")) != 1 {
		return api.NewToolCallResult("", fmt.Errorf("namespace is required when traceId is not provided")), nil
	}
	if resourceName == "" {
		return api.NewToolCallResult("", fmt.Errorf("resource_name is required when traceId is not provided")), nil
	}

	ops, ok := tracesOpsMap[resourceType]
	if !ok {
		return api.NewToolCallResult("", fmt.Errorf("invalid resource type: %s", resourceType)), nil
	}

	queryParams := make(map[string]string)

	// Handle startMicros: if not provided, default to 10 minutes ago
	var startMicros string
	if startMicrosVal, ok := params.GetArguments()["startMicros"].(string); ok && startMicrosVal != "" {
		startMicros = startMicrosVal
	} else {
		// Default to 10 minutes before current time
		now := time.Now()
		tenMinutesAgo := now.Add(-10 * time.Minute)
		startMicros = strconv.FormatInt(tenMinutesAgo.UnixMicro(), 10)
	}
	queryParams["startMicros"] = startMicros

	// Handle endMicros: if not provided, default to 10 minutes after startMicros
	var endMicros string
	if endMicrosVal, ok := params.GetArguments()["endMicros"].(string); ok && endMicrosVal != "" {
		endMicros = endMicrosVal
	} else {
		// Parse startMicros to calculate endMicros
		startMicrosInt, err := strconv.ParseInt(startMicros, 10, 64)
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("invalid startMicros value: %w", err)), nil
		}
		startTime := time.UnixMicro(startMicrosInt)
		endTime := startTime.Add(10 * time.Minute)
		endMicros = strconv.FormatInt(endTime.UnixMicro(), 10)
	}
	queryParams["endMicros"] = endMicros
	if err := setQueryParam(params, queryParams, "limit", kialiclient.DefaultLimit); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	// Handle minDuration: convert integer to string if provided
	if minDuration := params.GetArguments()["minDuration"]; minDuration != nil {
		if err := setQueryParam(params, queryParams, "minDuration", ""); err != nil {
			return api.NewToolCallResult("", err), nil
		}
	}
	if tags, ok := params.GetArguments()["tags"].(string); ok && tags != "" {
		queryParams["tags"] = tags
	}
	if clusterName, ok := params.GetArguments()["clusterName"].(string); ok && clusterName != "" {
		queryParams["clusterName"] = clusterName
	}
	content, err := ops.tracesFunc(params.Context, kiali, namespace, resourceName, queryParams)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get %s traces: %w", ops.singularName, err)), nil
	}
	return api.NewToolCallResult(content, nil), nil
}
