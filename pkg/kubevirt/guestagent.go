package kubevirt

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
)

const (
	// VMI subresource names for guest agent queries
	subresourceGuestOSInfo    = "guestosinfo"
	subresourceFilesystemList = "filesystemlist"
	subresourceUserList       = "userlist"
	subresourceInterfaceList  = "interfacelist"
)

var (
	// subresourcesScheme is the scheme for subresources API
	subresourcesScheme = Scheme
	// subresourcesCodec is the codec for encoding/decoding subresources
	subresourcesCodec = serializer.NewCodecFactory(subresourcesScheme)
)

// AllGuestInfo holds all guest agent information with error tracking
type AllGuestInfo struct {
	GuestOSInfo            any    `json:"guestOSInfo,omitempty" yaml:"guestOSInfo,omitempty"`
	GuestOSInfoError       string `json:"guestOSInfoError,omitempty" yaml:"guestOSInfoError,omitempty"`
	Filesystems            any    `json:"filesystems,omitempty" yaml:"filesystems,omitempty"`
	FilesystemsError       string `json:"filesystemsError,omitempty" yaml:"filesystemsError,omitempty"`
	Users                  any    `json:"users,omitempty" yaml:"users,omitempty"`
	UsersError             string `json:"usersError,omitempty" yaml:"usersError,omitempty"`
	NetworkInterfaces      any    `json:"networkInterfaces,omitempty" yaml:"networkInterfaces,omitempty"`
	NetworkInterfacesError string `json:"networkInterfacesError,omitempty" yaml:"networkInterfacesError,omitempty"`
}

// getVMISubresource retrieves a VMI subresource using the REST client
func getVMISubresource(ctx context.Context, restConfig *rest.Config, namespace, vmiName, subresource string) (map[string]any, error) {
	// Create a copy to avoid mutating the original config
	config := rest.CopyConfig(restConfig)

	// Create a REST client configured for the subresources.kubevirt.io API group
	gv := schema.GroupVersion{Group: "subresources.kubevirt.io", Version: "v1"}
	config.GroupVersion = &gv
	config.APIPath = "/apis"
	config.NegotiatedSerializer = subresourcesCodec.WithoutConversion()

	restClient, err := rest.RESTClientFor(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create REST client for subresources: %w", err)
	}

	// Make the request using SubResource() to properly construct the URL
	result := &unstructured.Unstructured{}
	err = restClient.Get().
		Namespace(namespace).
		Resource("virtualmachineinstances").
		Name(vmiName).
		SubResource(subresource).
		Do(ctx).
		Into(result)

	if err != nil {
		return nil, err
	}

	return result.Object, nil
}

// GetGuestOSInfo retrieves operating system information from the guest agent
func GetGuestOSInfo(ctx context.Context, restConfig *rest.Config, namespace, name string) (map[string]any, error) {
	result, err := getVMISubresource(ctx, restConfig, namespace, name, subresourceGuestOSInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to get guest OS info - guest agent may not be installed or running: %w", err)
	}

	return map[string]any{
		"guestOSInfo": result,
	}, nil
}

// GetFilesystemInfo retrieves filesystem and disk information from the guest agent
func GetFilesystemInfo(ctx context.Context, restConfig *rest.Config, namespace, name string) (map[string]any, error) {
	result, err := getVMISubresource(ctx, restConfig, namespace, name, subresourceFilesystemList)
	if err != nil {
		return nil, fmt.Errorf("failed to get filesystem info - guest agent may not be installed or running: %w", err)
	}

	return map[string]any{
		"filesystems": result,
	}, nil
}

// GetUserInfo retrieves logged-in user information from the guest agent
func GetUserInfo(ctx context.Context, restConfig *rest.Config, namespace, name string) (map[string]any, error) {
	result, err := getVMISubresource(ctx, restConfig, namespace, name, subresourceUserList)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info - guest agent may not be installed or running: %w", err)
	}

	return map[string]any{
		"users": result,
	}, nil
}

// GetNetworkInfo retrieves network interface information from the guest agent
func GetNetworkInfo(ctx context.Context, restConfig *rest.Config, namespace, name string) (map[string]any, error) {
	result, err := getVMISubresource(ctx, restConfig, namespace, name, subresourceInterfaceList)
	if err != nil {
		return nil, fmt.Errorf("failed to get network interface info - guest agent may not be installed or running: %w", err)
	}

	return map[string]any{
		"networkInterfaces": result,
	}, nil
}

// collectGuestOSInfo collects OS information and updates the result struct
func collectGuestOSInfo(ctx context.Context, restConfig *rest.Config, namespace, name string, result *AllGuestInfo) (bool, error) {
	osInfo, err := GetGuestOSInfo(ctx, restConfig, namespace, name)
	if err != nil {
		result.GuestOSInfoError = err.Error()
		return false, fmt.Errorf("guestOSInfo: %v", err)
	}
	result.GuestOSInfo = osInfo["guestOSInfo"]
	return true, nil
}

// collectFilesystemInfo collects filesystem information and updates the result struct
func collectFilesystemInfo(ctx context.Context, restConfig *rest.Config, namespace, name string, result *AllGuestInfo) (bool, error) {
	fsInfo, err := GetFilesystemInfo(ctx, restConfig, namespace, name)
	if err != nil {
		result.FilesystemsError = err.Error()
		return false, fmt.Errorf("filesystems: %v", err)
	}
	result.Filesystems = fsInfo["filesystems"]
	return true, nil
}

// collectUserInfo collects user information and updates the result struct
func collectUserInfo(ctx context.Context, restConfig *rest.Config, namespace, name string, result *AllGuestInfo) (bool, error) {
	userInfo, err := GetUserInfo(ctx, restConfig, namespace, name)
	if err != nil {
		result.UsersError = err.Error()
		return false, fmt.Errorf("users: %v", err)
	}
	result.Users = userInfo["users"]
	return true, nil
}

// collectNetworkInfo collects network information and updates the result struct
func collectNetworkInfo(ctx context.Context, restConfig *rest.Config, namespace, name string, result *AllGuestInfo) (bool, error) {
	netInfo, err := GetNetworkInfo(ctx, restConfig, namespace, name)
	if err != nil {
		result.NetworkInterfacesError = err.Error()
		return false, fmt.Errorf("networkInterfaces: %v", err)
	}
	result.NetworkInterfaces = netInfo["networkInterfaces"]
	return true, nil
}

// convertToMap converts AllGuestInfo struct to a map for consistent return type
func convertToMap(result *AllGuestInfo) map[string]any {
	resultMap := make(map[string]any)

	if result.GuestOSInfo != nil {
		resultMap["guestOSInfo"] = result.GuestOSInfo
	} else if result.GuestOSInfoError != "" {
		resultMap["guestOSInfoError"] = result.GuestOSInfoError
	}

	if result.Filesystems != nil {
		resultMap["filesystems"] = result.Filesystems
	} else if result.FilesystemsError != "" {
		resultMap["filesystemsError"] = result.FilesystemsError
	}

	if result.Users != nil {
		resultMap["users"] = result.Users
	} else if result.UsersError != "" {
		resultMap["usersError"] = result.UsersError
	}

	if result.NetworkInterfaces != nil {
		resultMap["networkInterfaces"] = result.NetworkInterfaces
	} else if result.NetworkInterfacesError != "" {
		resultMap["networkInterfacesError"] = result.NetworkInterfacesError
	}

	return resultMap
}

// GetAllGuestInfo retrieves all available guest agent information
func GetAllGuestInfo(ctx context.Context, restConfig *rest.Config, namespace, name string) (map[string]any, error) {
	result := &AllGuestInfo{}
	var errors []error
	successCount := 0

	// Collect all info types, but don't fail if one is unavailable
	if success, err := collectGuestOSInfo(ctx, restConfig, namespace, name, result); success {
		successCount++
	} else if err != nil {
		errors = append(errors, err)
	}

	if success, err := collectFilesystemInfo(ctx, restConfig, namespace, name, result); success {
		successCount++
	} else if err != nil {
		errors = append(errors, err)
	}

	if success, err := collectUserInfo(ctx, restConfig, namespace, name, result); success {
		successCount++
	} else if err != nil {
		errors = append(errors, err)
	}

	if success, err := collectNetworkInfo(ctx, restConfig, namespace, name, result); success {
		successCount++
	} else if err != nil {
		errors = append(errors, err)
	}

	// If all failed, return an aggregated error
	if successCount == 0 {
		return nil, fmt.Errorf("guest agent is not responding - all queries failed: %v - ensure QEMU guest agent is installed and running in the VM", errors)
	}

	return convertToMap(result), nil
}
