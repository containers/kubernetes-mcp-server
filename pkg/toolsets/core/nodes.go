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
						Type:        "string",
						Description: `query specifies services(s) or files from which to return logs (required). Example: "kubelet" to fetch kubelet logs, "/<log-file-name>" to fetch a specific log file from the node (e.g., "/var/log/kubelet.log" or "/var/log/kube-proxy.log")`,
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
	query, ok := params.GetArguments()["query"].(string)
	if !ok || query == "" {
		return api.NewToolCallResult("", errors.New("failed to get node log, missing argument query")), nil
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
	ret, err := params.NodesLog(params, name, query, tailInt)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get node log for %s: %v", name, err)), nil
	} else if ret == "" {
		ret = fmt.Sprintf("The node %s has not logged any message yet or the log file is empty", name)
	}
	return api.NewToolCallResult(ret, nil), nil
}
