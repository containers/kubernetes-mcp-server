package core

import (
	"bytes"
	"context"
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
)

func initNodes(ctx context.Context, inspector api.ClusterInspector) []api.ServerTool {
	return []api.ServerTool{
		{Tool: api.Tool{
			Name:        "nodes_log",
			Description: "Read the system logs of a node through the kubelet: a service such as kubelet, or a log file under /var/log. The last 100 lines by default; tail 0 returns everything.",
			Examples:    []api.Example{{Summary: "Kubelet logs of a node", Arguments: map[string]any{"name": "worker-1", "source": "kubelet"}}, {Summary: "A log file on a node", Arguments: map[string]any{"name": "worker-1", "source": "/var/log/kube-proxy.log", "tail": 50}}},
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"name": {
						Type:        "string",
						Description: "Name of the node to get logs from",
					},
					"source": {
						Type:        "string",
						Description: "A service name ('kubelet', 'kube-proxy') or a log file path on the node ('/var/log/kubelet.log').",
					},
					"tail": {
						Type:        "integer",
						Description: "Number of lines to retrieve from the end of the logs (Optional, 0 means all logs)",
						Default:     api.ToRawMessage(100),
						Minimum:     ptr.To(float64(0)),
					},
				},
				Required: []string{"name", "source"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "Node: Log",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, RBAC: api.RBACBounded(
			api.RBACRequirement{
				Verbs:        []string{"get"},
				Target:       api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "nodes"}},
				ResourceName: &api.RBACResourceName{Argument: "name"},
			},
			api.RBACRequirement{
				Verbs: []string{"get"},
				Target: api.RBACTarget{Resource: &api.RBACResourceTarget{
					Resource:    "nodes",
					Subresource: "proxy",
				}},
				ResourceName: &api.RBACResourceName{Argument: "name"},
			},
		), Handler: nodesLog},
		{Tool: api.Tool{
			Name:        "nodes_stats_summary",
			Description: "Show the kubelet's resource statistics of a node: CPU, memory, filesystem and network at the node, pod and container level, system containers included. nodes_top is the short CPU and memory table.",
			Examples:    []api.Example{{Summary: "Resource statistics of a node", Arguments: map[string]any{"name": "worker-1"}}},
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
		}, RBAC: api.RBACBounded(
			api.RBACRequirement{
				Verbs:        []string{"get"},
				Target:       api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "nodes"}},
				ResourceName: &api.RBACResourceName{Argument: "name"},
			},
			api.RBACRequirement{
				Verbs: []string{"get"},
				Target: api.RBACTarget{Resource: &api.RBACResourceTarget{
					Resource:    "nodes",
					Subresource: "proxy",
				}},
				ResourceName: &api.RBACResourceName{Argument: "name"},
			},
		), Handler: nodesStatsSummary},
		{Tool: api.Tool{
			Name:        "nodes_top",
			Description: "Show the CPU and memory consumption of the nodes as recorded by the Metrics Server: every node, one node with name, or the nodes matching labelSelector.",
			Examples:    []api.Example{{Summary: "CPU and memory of every node"}, {Summary: "Of the worker nodes", Arguments: map[string]any{"labelSelector": "node-role.kubernetes.io/worker="}}},
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"name": {
						Type:        "string",
						Description: "Name of the Node to get the resource consumption from (Optional, all Nodes if not provided)",
					},
					"labelSelector": {
						Type:        "string",
						Description: "Kubernetes label selector (e.g. 'node-role.kubernetes.io/worker=') to filter nodes by label (Optional, only applicable when name is not provided)",
						Pattern:     REGEX_LABELSELECTOR_VALID_CHARS,
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
		}, RBAC: api.RBACBounded(
			api.RBACRequirement{
				Verbs: []string{"get", "list"},
				Target: api.RBACTarget{Resource: &api.RBACResourceTarget{
					APIGroup: "metrics.k8s.io",
					Resource: "nodes",
				}},
			},
			api.RBACRequirement{
				Verbs:  []string{"list"},
				Target: api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "nodes"}},
			},
		), Handler: nodesTop,
			TargetCompatibilityFilters: []func() bool{
				kubernetes.HasNodeMetrics(ctx, inspector),
			},
		},
	}
}

func nodesLog(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	name, ok := params.GetArguments()["name"].(string)
	if !ok || name == "" {
		return api.NewToolCallResult("", errors.New("failed to get node log, missing argument name")), nil
	}
	query, ok := params.GetArguments()["source"].(string)
	if !ok || query == "" {
		return api.NewToolCallResult("", errors.New("failed to get node log, missing argument source")), nil
	}
	tail := params.GetArguments()["tail"]
	var tailInt int64
	if tail != nil {
		var err error
		tailInt, err = api.ParseInt64(tail)
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("failed to parse tail parameter: %w", err)), nil
		}
	}
	ret, err := kubernetes.NewCore(params).NodesLog(params, name, query, tailInt)
	if err != nil {
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
		return api.NewToolCallResult("", fmt.Errorf("failed to get node stats summary for %s: %w", name, err)), nil
	}
	return api.NewToolCallResult(ret, nil), nil
}

func nodesTop(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	nodesTopOptions := api.NodesTopOptions{}
	if v, ok := params.GetArguments()["name"].(string); ok {
		nodesTopOptions.Name = v
	}
	if v, ok := params.GetArguments()["labelSelector"].(string); ok {
		nodesTopOptions.LabelSelector = v
	}

	nodeMetrics, err := kubernetes.NewCore(params).NodesTop(params, nodesTopOptions)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get nodes top: %w", err)), nil
	}

	// Get the list of nodes to extract their allocatable resources
	nodeList, err := params.CoreV1().Nodes().List(params, metav1.ListOptions{
		LabelSelector: nodesTopOptions.LabelSelector,
	})
	if err != nil {
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
