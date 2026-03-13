package guestagent

import (
	"context"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubevirt"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/google/jsonschema-go/jsonschema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
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

func guestInfo(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Parse input parameters
	namespace, err := api.RequiredString(params, "namespace")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	name, err := api.RequiredString(params, "name")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	infoType := api.OptionalString(params, "info_type", "all")

	dynamicClient := params.DynamicClient()
	ctx := params.Context

	// First, check if the VMI exists
	vmi, err := dynamicClient.Resource(kubevirt.VirtualMachineInstanceGVR).
		Namespace(namespace).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("VirtualMachineInstance not found - VM may not be running: %w", err)), nil
	}

	// Check VMI status to see if it's running
	phase, found, err := unstructured.NestedString(vmi.Object, "status", "phase")
	if err != nil || !found || phase != "Running" {
		return api.NewToolCallResult("", fmt.Errorf("VirtualMachineInstance is not running (phase: %s) - guest agent requires VM to be running", phase)), nil
	}

	// Gather guest agent information based on info_type
	var result map[string]any
	switch GuestAgentInfoType(infoType) {
	case InfoTypeOS:
		result, err = getGuestOSInfo(ctx, dynamicClient, namespace, name)
	case InfoTypeFilesystem:
		result, err = getFilesystemInfo(ctx, dynamicClient, namespace, name)
	case InfoTypeUsers:
		result, err = getUserInfo(ctx, dynamicClient, namespace, name)
	case InfoTypeNetwork:
		result, err = getNetworkInfo(ctx, dynamicClient, namespace, name)
	case InfoTypeAll:
		result, err = getAllGuestInfo(ctx, dynamicClient, namespace, name)
	default:
		return api.NewToolCallResult("", fmt.Errorf("invalid info_type '%s': must be one of 'all', 'os', 'filesystem', 'users', 'network'", infoType)), nil
	}

	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	// Format the output
	marshalledYaml, err := output.MarshalYaml(result)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to marshal guest agent info: %w", err)), nil
	}

	message := fmt.Sprintf("# Guest Agent Information for VM: %s/%s\n\n", namespace, name)
	if infoType != "all" {
		message += fmt.Sprintf("**Info Type:** %s\n\n", infoType)
	}

	return api.NewToolCallResult(message+marshalledYaml, nil), nil
}

// getGuestOSInfo retrieves operating system information from the guest agent
func getGuestOSInfo(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string) (map[string]any, error) {
	gvr := schema.GroupVersionResource{
		Group:    "subresources.kubevirt.io",
		Version:  "v1",
		Resource: "virtualmachineinstances",
	}

	result, err := dynamicClient.Resource(gvr).
		Namespace(namespace).
		Get(ctx, name+"/guestosinfo", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get guest OS info - guest agent may not be installed or running: %w", err)
	}

	return map[string]any{
		"guestOSInfo": result.Object,
	}, nil
}

// getFilesystemInfo retrieves filesystem and disk information from the guest agent
func getFilesystemInfo(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string) (map[string]any, error) {
	gvr := schema.GroupVersionResource{
		Group:    "subresources.kubevirt.io",
		Version:  "v1",
		Resource: "virtualmachineinstances",
	}

	result, err := dynamicClient.Resource(gvr).
		Namespace(namespace).
		Get(ctx, name+"/filesystemlist", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get filesystem info - guest agent may not be installed or running: %w", err)
	}

	return map[string]any{
		"filesystems": result.Object,
	}, nil
}

// getUserInfo retrieves logged-in user information from the guest agent
func getUserInfo(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string) (map[string]any, error) {
	gvr := schema.GroupVersionResource{
		Group:    "subresources.kubevirt.io",
		Version:  "v1",
		Resource: "virtualmachineinstances",
	}

	result, err := dynamicClient.Resource(gvr).
		Namespace(namespace).
		Get(ctx, name+"/userlist", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get user info - guest agent may not be installed or running: %w", err)
	}

	return map[string]any{
		"users": result.Object,
	}, nil
}

// getNetworkInfo retrieves network interface information from the guest agent
func getNetworkInfo(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string) (map[string]any, error) {
	gvr := schema.GroupVersionResource{
		Group:    "subresources.kubevirt.io",
		Version:  "v1",
		Resource: "virtualmachineinstances",
	}

	result, err := dynamicClient.Resource(gvr).
		Namespace(namespace).
		Get(ctx, name+"/interfacelist", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get network interface info - guest agent may not be installed or running: %w", err)
	}

	return map[string]any{
		"networkInterfaces": result.Object,
	}, nil
}

// getAllGuestInfo retrieves all available guest agent information
func getAllGuestInfo(ctx context.Context, dynamicClient dynamic.Interface, namespace, name string) (map[string]any, error) {
	result := make(map[string]any)

	// Collect all info types, but don't fail if one is unavailable
	osInfo, err := getGuestOSInfo(ctx, dynamicClient, namespace, name)
	if err == nil {
		for k, v := range osInfo {
			result[k] = v
		}
	} else {
		result["guestOSInfo"] = map[string]string{"error": err.Error()}
	}

	fsInfo, err := getFilesystemInfo(ctx, dynamicClient, namespace, name)
	if err == nil {
		for k, v := range fsInfo {
			result[k] = v
		}
	} else {
		result["filesystems"] = map[string]string{"error": err.Error()}
	}

	userInfo, err := getUserInfo(ctx, dynamicClient, namespace, name)
	if err == nil {
		for k, v := range userInfo {
			result[k] = v
		}
	} else {
		result["users"] = map[string]string{"error": err.Error()}
	}

	netInfo, err := getNetworkInfo(ctx, dynamicClient, namespace, name)
	if err == nil {
		for k, v := range netInfo {
			result[k] = v
		}
	} else {
		result["networkInterfaces"] = map[string]string{"error": err.Error()}
	}

	// If all failed, return an error
	if len(result) == 4 &&
		result["guestOSInfo"] != nil &&
		result["filesystems"] != nil &&
		result["users"] != nil &&
		result["networkInterfaces"] != nil {
		return nil, fmt.Errorf("guest agent is not responding - ensure QEMU guest agent is installed and running in the VM")
	}

	return result, nil
}
