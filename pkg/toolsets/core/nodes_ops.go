package core

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

func initNodeOps() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "nodes_cordon",
				Description: "Cordon a Kubernetes Node, marking it as unschedulable so no new pods are placed on it. Existing pods are not evicted. Mirrors `kubectl cordon`",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"name": {
							Type:        "string",
							Description: "Name of the Node to cordon",
						},
					},
					Required: []string{"name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Node: Cordon",
					DestructiveHint: ptr.To(true),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: nodesCordon,
		},
		{
			Tool: api.Tool{
				Name:        "nodes_uncordon",
				Description: "Uncordon a Kubernetes Node, marking it as schedulable again so new pods can be placed on it. Mirrors `kubectl uncordon`",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"name": {
							Type:        "string",
							Description: "Name of the Node to uncordon",
						},
					},
					Required: []string{"name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Node: Uncordon",
					DestructiveHint: ptr.To(true),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: nodesUncordon,
		},
		{
			Tool: api.Tool{
				Name:        "nodes_drain",
				Description: "Drain a Kubernetes Node by cordoning it and evicting all its pods. Mirrors `kubectl drain`. Use caution: this evicts running workloads. DaemonSet pods and mirror pods are skipped by default when ignore_all_daemonsets is true",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"name": {
							Type:        "string",
							Description: "Name of the Node to drain",
						},
						"force": {
							Type:        "boolean",
							Description: "If true, continue even if there are pods not managed by a ReplicationController, ReplicaSet, Job, DaemonSet, or StatefulSet (Optional, default: false)",
							Default:     api.ToRawMessage(false),
						},
						"ignore_all_daemonsets": {
							Type:        "boolean",
							Description: "If true, ignore DaemonSet-managed pods and continue draining (Optional, default: true)",
							Default:     api.ToRawMessage(true),
						},
						"delete_emptydir_data": {
							Type:        "boolean",
							Description: "If true, continue even if pods are using emptyDir volumes (Optional, default: false)",
							Default:     api.ToRawMessage(false),
						},
						"disable_eviction": {
							Type:        "boolean",
							Description: "If true, force-delete pods instead of evicting them. Use only if eviction is blocked (Optional, default: false)",
							Default:     api.ToRawMessage(false),
						},
						"grace_period_seconds": {
							Type:        "integer",
							Description: "Period of time in seconds given to each pod to terminate gracefully. If negative, the default value specified in the pod will be used (Optional, default: -1)",
							Default:     api.ToRawMessage(-1),
						},
						"timeout": {
							Type:        "integer",
							Description: "The length of time in seconds to wait before giving up on draining a node, zero means infinite (Optional, default: 0)",
							Default:     api.ToRawMessage(0),
						},
					},
					Required: []string{"name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Node: Drain",
					DestructiveHint: ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: nodesDrain,
		},
		{
			Tool: api.Tool{
				Name:        "nodes_patch_label",
				Description: "Set or remove a single label on a Kubernetes Node. Providing an empty value removes the label. Useful for managing scheduling constraints and node grouping",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"name": {
							Type:        "string",
							Description: "Name of the Node to patch",
						},
						"key": {
							Type:        "string",
							Description: "Label key to set or remove (e.g. 'node-role.kubernetes.io/worker')",
						},
						"value": {
							Type:        "string",
							Description: "Label value to set. An empty string removes the label",
						},
					},
					Required: []string{"name", "key"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Node: Patch Label",
					DestructiveHint: ptr.To(true),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: nodesPatchLabel,
		},
		{
			Tool: api.Tool{
				Name:        "nodes_patch_taint",
				Description: "Set or remove a single taint on a Kubernetes Node. Taints repel pods unless a pod tolerates them. Use effect values NoSchedule, PreferNoSchedule, or NoExecute",
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"name": {
							Type:        "string",
							Description: "Name of the Node to patch",
						},
						"key": {
							Type:        "string",
							Description: "Taint key (e.g. 'dedicated')",
						},
						"value": {
							Type:        "string",
							Description: "Taint value (e.g. 'gpu'). Optional when removing",
						},
						"effect": {
							Type:        "string",
							Description: "Taint effect: NoSchedule, PreferNoSchedule, or NoExecute",
							Enum:        []any{"NoSchedule", "PreferNoSchedule", "NoExecute"},
						},
						"remove": {
							Type:        "boolean",
							Description: "If true, remove the taint matching key/effect instead of adding it (Optional, default: false)",
							Default:     api.ToRawMessage(false),
						},
					},
					Required: []string{"name", "key", "effect"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Node: Patch Taint",
					DestructiveHint: ptr.To(true),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(true),
				},
			},
			Handler: nodesPatchTaint,
		},
	}
}

func nodesCordon(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	name := p.RequiredString("name")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to cordon node: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).NodeCordon(params.Context, name)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to cordon node %s: %w", name, err)), nil
	}
	return api.NewToolCallResult(ret, nil), nil
}

func nodesUncordon(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	name := p.RequiredString("name")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to uncordon node: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).NodeUncordon(params.Context, name)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to uncordon node %s: %w", name, err)), nil
	}
	return api.NewToolCallResult(ret, nil), nil
}

func nodesDrain(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	name := p.RequiredString("name")
	opts := kubernetes.NodeDrainOptions{
		Force:               p.OptionalBool("force", false),
		IgnoreAllDaemonSets: p.OptionalBool("ignore_all_daemonsets", true),
		DeleteEmptyDirData:  p.OptionalBool("delete_emptydir_data", false),
		DisableEviction:     p.OptionalBool("disable_eviction", false),
		GracePeriodSeconds:  int(p.OptionalInt64("grace_period_seconds", -1)),
		Timeout:             int(p.OptionalInt64("timeout", 0)),
	}
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to drain node: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).NodeDrain(params.Context, name, opts)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to drain node %s: %w", name, err)), nil
	}
	return api.NewToolCallResult(ret, nil), nil
}

func nodesPatchLabel(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	name := p.RequiredString("name")
	key := p.RequiredString("key")
	value := p.OptionalString("value", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to patch node label: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).NodePatchLabel(params.Context, name, key, value)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to patch label on node %s: %w", name, err)), nil
	}
	return api.NewToolCallResult(ret, nil), nil
}

func nodesPatchTaint(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	name := p.RequiredString("name")
	key := p.RequiredString("key")
	effect := p.RequiredString("effect")
	value := p.OptionalString("value", "")
	remove := p.OptionalBool("remove", false)
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to patch node taint: %w", err)), nil
	}
	ret, err := kubernetes.NewCore(params).NodePatchTaint(params.Context, name, key, value, effect, remove)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to patch taint on node %s: %w", name, err)), nil
	}
	return api.NewToolCallResult(ret, nil), nil
}
