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
							Description: "The lifecycle action to perform: 'start', 'stop', or 'restart'",
						},
						"run_policy": {
							Type: "string",
							Enum: []any{
								string(kubevirt.RunPolicyHighAvailability),
								string(kubevirt.RunPolicyRestartOnFailure),
								string(kubevirt.RunPolicyOnce),
							},
							Description: "The run policy to use when starting or restarting a VM (applies to 'start' and 'restart' actions; ignored for 'stop'). Options:\n" +
								"  - 'HighAvailability': VM runs continuously (sets runStrategy to Always)\n" +
								"  - 'RestartOnFailure': VM restarts on failure (sets runStrategy to RerunOnFailure)\n" +
								"  - 'Once': VM runs once and stops after completion (sets runStrategy to Once)\n" +
								"Defaults to 'HighAvailability' if not specified.",
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
		var wasStarted bool
		runPolicyStr := p.OptionalString("run_policy", string(kubevirt.RunPolicyHighAvailability))
		runPolicy := kubevirt.RunPolicy(runPolicyStr)
		if !kubevirt.IsValidRunPolicy(runPolicy) {
			return api.NewToolCallResult("", fmt.Errorf("invalid run policy '%s': must be one of 'HighAvailability', 'RestartOnFailure', 'Once'", runPolicyStr)), nil
		}
		vm, wasStarted, err = kubevirt.StartVM(params.Context, dynamicClient, namespace, name, runPolicy)
		if err != nil {
			return api.NewToolCallResult("", err), nil
		}
		if wasStarted {
			message = fmt.Sprintf("# VirtualMachine started successfully with run policy '%s'\n", runPolicy)
		} else {
			message = fmt.Sprintf("# VirtualMachine '%s' in namespace '%s' is already running with the desired run strategy\n", name, namespace)
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
		runPolicyStr := p.OptionalString("run_policy", string(kubevirt.RunPolicyHighAvailability))
		runPolicy := kubevirt.RunPolicy(runPolicyStr)
		if !kubevirt.IsValidRunPolicy(runPolicy) {
			return api.NewToolCallResult("", fmt.Errorf("invalid run policy '%s': must be one of 'HighAvailability', 'RestartOnFailure', 'Once'", runPolicyStr)), nil
		}
		vm, err = kubevirt.RestartVM(params.Context, dynamicClient, namespace, name, runPolicy)
		if err != nil {
			return api.NewToolCallResult("", err), nil
		}
		message = fmt.Sprintf("# VirtualMachine restarted successfully with run policy '%s'\n", runPolicy)

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
