package core

import (
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func initNodes() []api.ServerTool {
	return []api.ServerTool{
		{Tool: api.Tool{
			Name:        "nodes_log",
			Description: "Get logs from a Kubernetes node (kubelet, kube-proxy, or other system logs). This accesses node logs through the Kubernetes API proxy to the kubelet",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"name": {
						Type:        "string",
						Description: "Name of the node to get logs from",
					},
					"query": {
						OneOf: []*jsonschema.Schema{
							{
								Type:        "string",
								Description: `Single query specifying a service or file from which to return logs. Example: "kubelet" or "kubelet.log"`,
							},
							{
								Type:        "array",
								Description: `Array of queries specifying multiple services or files from which to return logs`,
								Items: &jsonschema.Schema{
									Type: "string",
								},
							},
						},
						Description: `query specifies service(s) or files from which to return logs (required). Can be a single string or array of strings. Example: "kubelet" to fetch kubelet logs, "/<log-file-name>" to fetch a specific log file from the node (e.g., "kubelet.log" or "kube-proxy.log"), or ["kubelet", "kube-proxy.log"] for multiple sources`,
					},
					"tailLines": {
						Type:        "integer",
						Description: "Number of lines to retrieve from the end of the logs (Optional, 0 means all logs)",
						Default:     api.ToRawMessage(100),
						Minimum:     ptr.To(float64(0)),
					},
				},
				Required: []string{"name", "query"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Node: Log",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: nodesLog},
	}
}

func nodesLog(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	name, ok := params.GetArguments()["name"].(string)
	if !ok || name == "" {
		return api.NewToolCallResult("", errors.New("failed to get node log, missing argument name")), nil
	}

	// Handle query parameter - can be string or array of strings
	var queries []string
	queryArg := params.GetArguments()["query"]
	if queryArg == nil {
		return api.NewToolCallResult("", errors.New("failed to get node log, missing argument query")), nil
	}

	switch v := queryArg.(type) {
	case string:
		if v == "" {
			return api.NewToolCallResult("", errors.New("failed to get node log, query cannot be empty")), nil
		}
		queries = []string{v}
	case []interface{}:
		if len(v) == 0 {
			return api.NewToolCallResult("", errors.New("failed to get node log, query array cannot be empty")), nil
		}
		for i, item := range v {
			str, ok := item.(string)
			if !ok {
				return api.NewToolCallResult("", fmt.Errorf("failed to get node log, query array element %d is not a string", i)), nil
			}
			if str == "" {
				return api.NewToolCallResult("", fmt.Errorf("failed to get node log, query array element %d cannot be empty", i)), nil
			}
			queries = append(queries, str)
		}
	default:
		return api.NewToolCallResult("", fmt.Errorf("failed to get node log, query must be a string or array of strings, got %T", queryArg)), nil
	}

	tailLines := params.GetArguments()["tailLines"]
	var tailInt int64
	if tailLines != nil {
		// Convert to int64 - safely handle both float64 (JSON number) and int types
		switch v := tailLines.(type) {
		case float64:
			tailInt = int64(v)
		case int:
		case int64:
			tailInt = v
		default:
			return api.NewToolCallResult("", fmt.Errorf("failed to parse tail parameter: expected integer, got %T", tailLines)), nil
		}
	}

	// Fetch logs for each query and concatenate results
	var results string
	for i, query := range queries {
		ret, err := params.NodesLog(params, name, query, tailInt)
		if err != nil {
			// Only include query in error message if there are multiple queries
			if len(queries) > 1 {
				return api.NewToolCallResult("", fmt.Errorf("failed to get node log for %s (query: %s): %v", name, query, err)), nil
			}
			return api.NewToolCallResult("", fmt.Errorf("failed to get node log for %s: %v", name, err)), nil
		}

		if len(queries) > 1 {
			// Add separator between multiple queries
			if i > 0 {
				results += "\n\n"
			}
			results += fmt.Sprintf("=== Logs for query: %s ===\n", query)
		}

		if ret == "" {
			if len(queries) > 1 {
				results += fmt.Sprintf("The node %s has not logged any message yet for query '%s' or the log file is empty\n", name, query)
			} else {
				results = fmt.Sprintf("The node %s has not logged any message yet or the log file is empty", name)
			}
		} else {
			results += ret
		}
	}

	if results == "" {
		results = fmt.Sprintf("The node %s has not logged any message yet or the log file is empty", name)
	}

	return api.NewToolCallResult(results, nil), nil
}
