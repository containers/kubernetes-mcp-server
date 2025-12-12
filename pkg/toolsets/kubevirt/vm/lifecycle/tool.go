package lifecycle

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubevirt"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
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
)

func Tools() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "vm_lifecycle",
				Description: "Manage VirtualMachine lifecycle: start, stop, or restart a VM",
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
							Enum:        []any{string(ActionStart), string(ActionStop), string(ActionRestart)},
							Description: "The lifecycle action to perform: 'start' (changes runStrategy to Always), 'stop' (changes runStrategy to Halted), or 'restart' (stops then starts the VM)",
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
		},
	}
}

func lifecycle(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Parse input parameters
	namespace, err := api.RequiredString(params, "namespace")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	name, err := api.RequiredString(params, "name")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	action, err := api.RequiredString(params, "action")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	dynamicClient := params.DynamicClient()

	var vm *unstructured.Unstructured
	var message string

	switch Action(action) {
	case ActionStart:
		var wasStarted bool
		vm, wasStarted, err = kubevirt.StartVM(params.Context, dynamicClient, namespace, name)
		if err != nil {
			return api.NewToolCallResult("", err), nil
		}
		if wasStarted {
			message = "# VirtualMachine started successfully\n"
		} else {
			message = fmt.Sprintf("# VirtualMachine '%s' in namespace '%s' is already running\n", name, namespace)
		}

	case ActionStop:
		var wasRunning bool
		vm, wasRunning, err = kubevirt.StopVM(params.Context, dynamicClient, namespace, name)
		if err != nil {
			return api.NewToolCallResult("", err), nil
		}
		if wasRunning {
			message = "# VirtualMachine stopped successfully\n"
		} else {
			message = fmt.Sprintf("# VirtualMachine '%s' in namespace '%s' is already stopped\n", name, namespace)
		}

	case ActionRestart:
		vm, err = kubevirt.RestartVM(params.Context, dynamicClient, namespace, name)
		if err != nil {
			return api.NewToolCallResult("", err), nil
		}
		message = "# VirtualMachine restarted successfully\n"

	default:
		return api.NewToolCallResult("", fmt.Errorf("invalid action '%s': must be one of 'start', 'stop', 'restart'", action)), nil
	}

	// Format the output
	marshalledYaml, err := output.MarshalYaml([]*unstructured.Unstructured{vm})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to marshal VirtualMachine: %w", err)), nil
	}

	return api.NewToolCallResult(message+marshalledYaml, nil), nil
}
