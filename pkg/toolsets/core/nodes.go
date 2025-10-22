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
			Name:        "node_log",
			Description: "Get logs from a Kubernetes node (kubelet, kube-proxy, or other system logs). This accesses node logs through the Kubernetes API proxy to the kubelet",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"name": {
						Type:        "string",
						Description: "Name of the node to get logs from",
					},
					"log_path": {
						Type:        "string",
						Description: "Path to the log file on the node (e.g. 'kubelet.log', 'kube-proxy.log'). Default is 'kubelet.log'",
						Default:     api.ToRawMessage("kubelet.log"),
					},
					"tail": {
						Type:        "integer",
						Description: "Number of lines to retrieve from the end of the logs (Optional, 0 means all logs)",
						Default:     api.ToRawMessage(100),
						Minimum:     ptr.To(float64(0)),
					},
				},
				Required: []string{"name"},
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
	name := params.GetArguments()["name"]
	if name == nil {
		return api.NewToolCallResult("", errors.New("failed to get node log, missing argument name")), nil
	}
	logPath := params.GetArguments()["log_path"]
	if logPath == nil {
		logPath = "kubelet.log"
	}
	tail := params.GetArguments()["tail"]
	var tailInt int64
	if tail != nil {
		// Convert to int64 - safely handle both float64 (JSON number) and int types
		switch v := tail.(type) {
		case float64:
			tailInt = int64(v)
		case int: case int64:
			tailInt = int64(v)
		default:
			return api.NewToolCallResult("", fmt.Errorf("failed to parse tail parameter: expected integer, got %T", tail)), nil
		}
	}
	ret, err := params.NodeLog(params, name.(string), logPath.(string), tailInt)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get node log for %s: %v", name, err)), nil
	} else if ret == "" {
		ret = fmt.Sprintf("The node %s has not logged any message yet or the log file is empty", name)
	}
	return api.NewToolCallResult(ret, nil), nil
}
