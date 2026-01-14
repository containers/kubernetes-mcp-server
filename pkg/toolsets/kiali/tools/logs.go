package tools

import (
	"fmt"

	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kiali/internal/defaults"
)

func InitLogs() []api.ServerTool {
	ret := make([]api.ServerTool, 0)
	name := defaults.ToolsetName() + "_workload_logs"
	// Workload logs tool
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        name,
			Description: "Get logs for a specific workload's pods in a namespace. Only requires namespace and workload name - automatically discovers pods and containers. Optionally filter by container name, time range, and other parameters. Container is auto-detected if not specified.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"namespace": {
						Type:        "string",
						Description: "Namespace containing the workload",
					},
					"workload": {
						Type:        "string",
						Description: "Name of the workload to get logs for",
					},
					"container": {
						Type:        "string",
						Description: "Optional container name to filter logs. If not provided, automatically detects and uses the main application container (excludes istio-proxy and istio-init)",
					},
					"since": {
						Type:        "string",
						Description: "Time duration to fetch logs from (e.g., '5m', '1h', '30s'). If not provided, returns recent logs",
					},
					"tail": {
						Type:        "integer",
						Description: "Number of lines to retrieve from the end of logs (default: 100)",
						Minimum:     ptr.To(float64(1)),
					},
				},
				Required: []string{"namespace", "workload"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Workload: Logs",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: workloadLogsHandler,
	})

	return ret
}

func workloadLogsHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Extract required parameters
	namespace, _ := params.GetArguments()["namespace"].(string)
	workload, _ := params.GetArguments()["workload"].(string)
	if namespace == "" {
		return api.NewToolCallResult("", fmt.Errorf("namespace parameter is required")), nil
	}
	if workload == "" {
		return api.NewToolCallResult("", fmt.Errorf("workload parameter is required")), nil
	}

	// Extract optional parameters
	container, _ := params.GetArguments()["container"].(string)
	since, _ := params.GetArguments()["since"].(string)
	tail := params.GetArguments()["tail"]

	// Convert parameters to Kiali API format
	var duration, maxLines string

	// Convert since to duration (Kiali expects duration format like "5m", "1h")
	if since != "" {
		duration = since
	}

	// Convert tail to maxLines
	if tail != nil {
		tailInt, err := api.ParseInt64(tail)
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("failed to parse tail parameter: %w", err)), nil
		}
		maxLines = fmt.Sprintf("%d", tailInt)
	}

	// WorkloadLogs handles container auto-detection internally, so we can pass empty string
	// if container is not specified
	kiali := kialiclient.NewKiali(params, params.RESTConfig())
	logs, err := kiali.WorkloadLogs(params.Context, namespace, workload, container, duration, maxLines)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get workload logs: %w", err)), nil
	}

	return api.NewToolCallResult(logs, nil), nil
}
