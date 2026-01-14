package core

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubectl/pkg/metricsutil"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/mcplog"
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
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: nodesLog},
		{Tool: api.Tool{
			Name:        "nodes_stats_summary",
			Description: "Get detailed resource usage statistics from a Kubernetes node via the kubelet's Summary API. Provides comprehensive metrics including CPU, memory, filesystem, and network usage at the node, pod, and container levels. On systems with cgroup v2 and kernel 4.20+, also includes PSI (Pressure Stall Information) metrics that show resource pressure for CPU, memory, and I/O. See https://kubernetes.io/docs/reference/instrumentation/understand-psi-metrics/ for details on PSI metrics",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"name": {
						Type:        "string",
						Description: "Name of the node to get stats from",
					},
				},
				Required: []string{"name"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Node: Stats Summary",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: nodesStatsSummary},
		{Tool: api.Tool{
			Name:        "nodes_top",
			Description: "List the resource consumption (CPU and memory) as recorded by the Kubernetes Metrics Server for the specified Kubernetes Nodes or all nodes in the cluster",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"name": {
						Type:        "string",
						Description: "Name of the Node to get the resource consumption from (Optional, all Nodes if not provided)",
					},
					"label_selector": {
						Type:        "string",
						Description: "Kubernetes label selector (e.g. 'node-role.kubernetes.io/worker=') to filter nodes by label (Optional, only applicable when name is not provided)",
						Pattern:     "([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]",
					},
				},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Nodes: Top",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				IdempotentHint:  ptr.To(true),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: nodesTop},
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
		var err error
		tailInt, err = api.ParseInt64(tailLines)
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("failed to parse tailLines parameter: %w", err)), nil
		}
	}
	ret, err := kubernetes.NewCore(params).NodesLog(params, name, query, tailInt)
	if err != nil {
		mcplog.HandleK8sError(params.Context, err, "node log access")
		return api.NewToolCallResult("", fmt.Errorf("failed to get node log for %s: %w", name, err)), nil
	} else if ret == "" {
		ret = fmt.Sprintf("The node %s has not logged any message yet or the log file is empty", name)
	}
	return api.NewToolCallResult(ret, nil), nil
}

func nodesStatsSummary(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	name, ok := params.GetArguments()["name"].(string)
	if !ok || name == "" {
		return api.NewToolCallResult("", errors.New("failed to get node stats summary, missing argument name")), nil
	}
	ret, err := kubernetes.NewCore(params).NodesStatsSummary(params, name)
	if err != nil {
		mcplog.HandleK8sError(params.Context, err, "node stats access")
		return api.NewToolCallResult("", fmt.Errorf("failed to get node stats summary for %s: %w", name, err)), nil
	}
	return api.NewToolCallResult(ret, nil), nil
}

func nodesTop(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	nodesTopOptions := api.NodesTopOptions{}
	if v, ok := params.GetArguments()["name"].(string); ok {
		nodesTopOptions.Name = v
	}
	if v, ok := params.GetArguments()["label_selector"].(string); ok {
		nodesTopOptions.LabelSelector = v
	}

	nodeMetrics, err := kubernetes.NewCore(params).NodesTop(params, nodesTopOptions)
	if err != nil {
		mcplog.HandleK8sError(params.Context, err, "node metrics access")
		return api.NewToolCallResult("", fmt.Errorf("failed to get nodes top: %w", err)), nil
	}

	// Get the list of nodes to extract their allocatable resources
	nodeList, err := params.CoreV1().Nodes().List(params, metav1.ListOptions{
		LabelSelector: nodesTopOptions.LabelSelector,
	})
	if err != nil {
		mcplog.HandleK8sError(params.Context, err, "node listing")
		return api.NewToolCallResult("", fmt.Errorf("failed to list nodes: %w", err)), nil
	}

	// Build availableResources map
	availableResources := make(map[string]v1.ResourceList)
	for _, n := range nodeList.Items {
		availableResources[n.Name] = n.Status.Allocatable

		// Handle swap if available
		if n.Status.NodeInfo.Swap != nil && n.Status.NodeInfo.Swap.Capacity != nil {
			swapCapacity := *n.Status.NodeInfo.Swap.Capacity
			availableResources[n.Name]["swap"] = *resource.NewQuantity(swapCapacity, resource.BinarySI)
		}
	}

	// Print the metrics
	buf := new(bytes.Buffer)
	printer := metricsutil.NewTopCmdPrinter(buf, true)
	err = printer.PrintNodeMetrics(nodeMetrics.Items, availableResources, false, "")
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to print node metrics: %w", err)), nil
	}

	return api.NewToolCallResult(buf.String(), nil), nil
}
