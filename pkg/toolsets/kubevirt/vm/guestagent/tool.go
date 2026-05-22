package guestagent

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubevirt"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/google/jsonschema-go/jsonschema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
)

// GuestAgentInfoType represents the type of information to retrieve from guest agent
type GuestAgentInfoType string

const (
	InfoTypeAll        GuestAgentInfoType = "all"
	InfoTypeOS         GuestAgentInfoType = "os"
	InfoTypeFilesystem GuestAgentInfoType = "filesystem"
	InfoTypeUsers      GuestAgentInfoType = "users"
	InfoTypeNetwork    GuestAgentInfoType = "network"
)

func Tools() []api.ServerTool {
	return []api.ServerTool{
		{
			Tool: api.Tool{
				Name:        "vm_guest_info",
				Description: "Get guest operating system information from a VirtualMachine's QEMU guest agent. Requires the guest agent to be installed and running inside the VM. Provides detailed information about the OS, filesystems, network interfaces, and logged-in users.",
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
						"info_type": {
							Type:        "string",
							Enum:        []any{"all", "os", "filesystem", "users", "network"},
							Description: "Type of information to retrieve: 'all' (default - all available info), 'os' (operating system details), 'filesystem' (disk and filesystem info), 'users' (logged-in users), 'network' (network interfaces and IPs)",
							Default:     api.ToRawMessage("all"),
						},
					},
					Required: []string{"namespace", "name"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Virtual Machine: Guest Agent Info",
					ReadOnlyHint:    ptr.To(true),
					DestructiveHint: ptr.To(false),
					IdempotentHint:  ptr.To(true),
					OpenWorldHint:   ptr.To(false),
				},
			},
			Handler: guestInfo,
		},
	}
}

// validateVMI checks if the VMI exists and is running
func validateVMI(params api.ToolHandlerParams, namespace, name string) error {
	vmi, err := params.DynamicClient().Resource(kubevirt.VirtualMachineInstanceGVR).
		Namespace(namespace).
		Get(params.Context, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("VirtualMachineInstance not found - VM may not be running: %w", err)
	}

	phase, found, err := unstructured.NestedString(vmi.Object, "status", "phase")
	if err != nil || !found || phase != "Running" {
		return fmt.Errorf("VirtualMachineInstance is not running (phase: %s) - guest agent requires VM to be running", phase)
	}

	return nil
}

// fetchGuestInfo retrieves the requested guest information based on info type
func fetchGuestInfo(params api.ToolHandlerParams, namespace, name, infoType string) (map[string]any, error) {
	ctx := params.Context
	restConfig := params.RESTConfig()

	switch GuestAgentInfoType(infoType) {
	case InfoTypeOS:
		return kubevirt.GetGuestOSInfo(ctx, restConfig, namespace, name)
	case InfoTypeFilesystem:
		return kubevirt.GetFilesystemInfo(ctx, restConfig, namespace, name)
	case InfoTypeUsers:
		return kubevirt.GetUserInfo(ctx, restConfig, namespace, name)
	case InfoTypeNetwork:
		return kubevirt.GetNetworkInfo(ctx, restConfig, namespace, name)
	case InfoTypeAll:
		return kubevirt.GetAllGuestInfo(ctx, restConfig, namespace, name)
	default:
		return nil, fmt.Errorf("invalid info_type '%s': must be one of 'all', 'os', 'filesystem', 'users', 'network'", infoType)
	}
}

// formatGuestInfoOutput formats the guest information as YAML output
func formatGuestInfoOutput(namespace, name, infoType string, result map[string]any) (string, error) {
	marshalledYaml, err := output.MarshalYaml(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal guest agent info: %w", err)
	}

	message := fmt.Sprintf("# Guest Agent Information for VM: %s/%s\n\n", namespace, name)
	if infoType != "all" {
		message += fmt.Sprintf("**Info Type:** %s\n\n", infoType)
	}

	return message + marshalledYaml, nil
}

func guestInfo(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	namespace, err := api.RequiredString(params, "namespace")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	name, err := api.RequiredString(params, "name")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	args := params.GetArguments()
	infoType := "all"
	if val, ok := args["info_type"]; ok && val != nil {
		s, ok := val.(string)
		if !ok {
			return api.NewToolCallResult("", fmt.Errorf("info_type parameter must be a string")), nil
		}
		infoType = s
	}

	if err := validateVMI(params, namespace, name); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	result, err := fetchGuestInfo(params, namespace, name, infoType)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	output, err := formatGuestInfoOutput(namespace, name, infoType, result)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	return api.NewToolCallResult(output, nil), nil
}
