package lifecycle

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubevirt"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/internal/defaults"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
)

// Action represents the lifecycle action to perform on a VM
type Action string

const (
	ActionStart   Action = "start"
	ActionStop    Action = "stop"
	ActionRestart Action = "restart"
	ActionPause   Action = "pause"
	ActionUnpause Action = "unpause"
)

func Tools(p api.FilteringProvider) []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "vm_lifecycle",
				Description: fmt.Sprintf("Manage %s VirtualMachine lifecycle: start, stop, restart, pause, or unpause a VM", defaults.ProductName()),
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"namespace": {
							Type:        "string",
							Description: "The namespace of the virtual machine",
						},
						"name": {
							Type:        "string",
							Description: "The name of the virtual machine",
						},
						"action": {
							Type:        "string",
							Enum:        []any{string(ActionStart), string(ActionStop), string(ActionRestart), string(ActionPause), string(ActionUnpause)},
							Description: "The lifecycle action to perform: 'start' (sets runStrategy from run_policy), 'stop' (sets runStrategy to Halted), 'restart' (stops then starts; see run_policy), 'pause' (suspends the running VMI in-place), or 'unpause' (resumes a paused VMI)",
						},
						"run_policy": {
							Type: "string",
							Enum: []any{
								string(kubevirt.RunPolicyHighAvailability),
								string(kubevirt.RunPolicyRestartOnFailure),
								string(kubevirt.RunPolicyOnce),
							},
							Description: "The run policy for 'start' and 'restart' (ignored for 'stop', 'pause', and 'unpause'): " +
								"'HighAvailability' (Always), 'RestartOnFailure' (RerunOnFailure), or 'Once' (Once). " +
								"Defaults to 'HighAvailability'.",
						},
					},
					Required: []string{"namespace", "name", "action"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Virtual Machine: Lifecycle",
					ReadOnlyHint:    ptr.To(false),
					DestructiveHint: ptr.To(true),
					IdempotentHint:  ptr.To(false),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: lifecycle,
			TargetCompatibilityFilters: []func() bool{
				kubevirt.HasVirtualMachine(p),
			},
		},
	}
}

func lifecycle(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Parse input parameters
	p := api.WrapParams(params)
	namespace := p.RequiredString("namespace")
	name := p.RequiredString("name")
	action := p.RequiredString("action")

	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	dynamicClient := params.DynamicClient()

	var vm *unstructured.Unstructured
	var message string
	var err error

	switch Action(action) {
	case ActionStart:
		var wasStarted, strategyChanged bool
		runPolicy := kubevirt.RunPolicy(p.OptionalString("run_policy", string(kubevirt.RunPolicyHighAvailability)))
		vm, wasStarted, strategyChanged, err = kubevirt.StartVM(params.Context, dynamicClient, namespace, name, runPolicy)
		if err != nil {
			return api.NewToolCallResult("", err), nil
		}
		switch {
		case wasStarted:
			message = fmt.Sprintf("# VirtualMachine started successfully with run policy '%s'\n", runPolicy)
		case strategyChanged:
			message = fmt.Sprintf("# VirtualMachine run strategy updated to run policy '%s'\n", runPolicy)
		default:
			message = fmt.Sprintf("# VirtualMachine '%s' in namespace '%s' is already running with the desired run strategy\n", name, namespace)
		}

	case ActionStop:
		var wasStopped bool
		vm, wasStopped, err = kubevirt.StopVM(params.Context, dynamicClient, namespace, name)
		if err != nil {
			return api.NewToolCallResult("", err), nil
		}
		if wasStopped {
			message = "# VirtualMachine stopped successfully\n"
		} else {
			message = fmt.Sprintf("# VirtualMachine '%s' in namespace '%s' is already stopped\n", name, namespace)
		}

	case ActionRestart:
		runPolicy := kubevirt.RunPolicy(p.OptionalString("run_policy", string(kubevirt.RunPolicyHighAvailability)))
		vm, err = kubevirt.RestartVM(params.Context, dynamicClient, namespace, name, runPolicy)
		if err != nil {
			return api.NewToolCallResult("", err), nil
		}
		message = fmt.Sprintf("# VirtualMachine restarted successfully with run policy '%s'\n", runPolicy)

	case ActionPause:
		vm, err = kubevirt.PauseVM(params.Context, dynamicClient, params.RESTConfig(), namespace, name)
		if err != nil {
			return api.NewToolCallResult("", err), nil
		}
		message = "# VirtualMachine paused successfully\n"

	case ActionUnpause:
		vm, err = kubevirt.UnpauseVM(params.Context, dynamicClient, params.RESTConfig(), namespace, name)
		if err != nil {
			return api.NewToolCallResult("", err), nil
		}
		message = "# VirtualMachine unpaused successfully\n"

	default:
		return api.NewToolCallResult("", fmt.Errorf("invalid action '%s': must be one of 'start', 'stop', 'restart', 'pause', 'unpause'", action)), nil
	}

	// Format the output
	marshalledYaml, err := output.MarshalYaml([]*unstructured.Unstructured{vm})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to marshal VirtualMachine: %w", err)), nil
	}

	return api.NewToolCallResult(message+marshalledYaml, nil), nil
}
