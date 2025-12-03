package kiali

import (
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func initLogs() []api.ServerTool {
	ret := make([]api.ServerTool, 0)

	// Workload logs tool
	ret = append(ret, api.ServerTool{
		Tool: api.Tool{
			Name:        "workload_logs",
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
	k := params.NewKiali()
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
	var duration, logType, sinceTime, maxLines string
	var service string // We don't have service parameter in our schema, but Kiali API supports it

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

	// If no container specified, we need to get workload details first to find the main app container
	if container == "" {
		workloadDetails, err := k.WorkloadDetails(params.Context, namespace, workload)
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("failed to get workload details: %v", err)), nil
		}

		// Parse the workload details JSON to extract container names
		var workloadData struct {
			Pods []struct {
				Name       string `json:"name"`
				Containers []struct {
					Name string `json:"name"`
				} `json:"containers"`
			} `json:"pods"`
		}

		if err := json.Unmarshal([]byte(workloadDetails), &workloadData); err != nil {
			return api.NewToolCallResult("", fmt.Errorf("failed to parse workload details: %v", err)), nil
		}

		if len(workloadData.Pods) == 0 {
			return api.NewToolCallResult("", fmt.Errorf("no pods found for workload %s in namespace %s", workload, namespace)), nil
		}

		// Find the main application container (not istio-proxy or istio-init)
		for _, pod := range workloadData.Pods {
			for _, c := range pod.Containers {
				if c.Name != "istio-proxy" && c.Name != "istio-init" {
					container = c.Name
					break
				}
			}
			if container != "" {
				break
			}
		}

		// If no app container found, use the first container
		if container == "" && len(workloadData.Pods) > 0 && len(workloadData.Pods[0].Containers) > 0 {
			container = workloadData.Pods[0].Containers[0].Name
		}
	}

	if container == "" {
		return api.NewToolCallResult("", fmt.Errorf("no container found for workload %s in namespace %s", workload, namespace)), nil
	}

	// Use the WorkloadLogs method with the correct parameters
	logs, err := k.WorkloadLogs(params.Context, namespace, workload, container, service, duration, logType, sinceTime, maxLines)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get workload logs: %v", err)), nil
	}

	return api.NewToolCallResult(logs, nil), nil
}
