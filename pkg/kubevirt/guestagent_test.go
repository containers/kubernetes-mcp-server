package kubevirt

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
)

// createTestRESTConfig creates a REST config pointing to a test HTTP server
func createTestRESTConfig(server *httptest.Server) *rest.Config {
	return &rest.Config{
		Host: server.URL,
	}
}

// createGuestOSInfoResponse creates a fake guest OS info response
func createGuestOSInfoResponse() map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "subresources.kubevirt.io/v1",
		"kind":       "VirtualMachineInstanceGuestOSInfo",
		"guestOSInfo": map[string]interface{}{
			"name":          "Fedora Linux",
			"version":       "40",
			"id":            "fedora",
			"kernelRelease": "6.8.5-301.fc40.x86_64",
		},
	}
}

// createFilesystemListResponse creates a fake filesystem list response
func createFilesystemListResponse() map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "subresources.kubevirt.io/v1",
		"kind":       "VirtualMachineInstanceFilesystemList",
		"items": []interface{}{
			map[string]interface{}{
				"diskName":       "vda",
				"mountPoint":     "/",
				"fileSystemType": "ext4",
				"usedBytes":      1073741824,
				"totalBytes":     10737418240,
			},
		},
	}
}

// createUserListResponse creates a fake user list response
func createUserListResponse() map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "subresources.kubevirt.io/v1",
		"kind":       "VirtualMachineInstanceGuestOSUserList",
		"items": []interface{}{
			map[string]interface{}{
				"userName":  "fedora",
				"loginTime": 1234567890.0,
			},
		},
	}
}

// createInterfaceListResponse creates a fake network interface list response
func createInterfaceListResponse() map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "subresources.kubevirt.io/v1",
		"kind":       "VirtualMachineInstanceGuestOSInterfaceList",
		"items": []interface{}{
			map[string]interface{}{
				"interfaceName": "eth0",
				"ipAddresses":   []interface{}{"10.0.2.15", "fe80::5054:ff:fe12:3456"},
				"mac":           "52:54:00:12:34:56",
			},
		},
	}
}

func TestGetGuestOSInfo(t *testing.T) {
	testGuestInfoFunction(
		t,
		GetGuestOSInfo,
		"/apis/subresources.kubevirt.io/v1/namespaces/default/virtualmachineinstances/test-vm/guestosinfo",
		createGuestOSInfoResponse,
		"guestOSInfo",
		"failed to get guest OS info",
	)
}

func TestGetFilesystemInfo(t *testing.T) {
	testGuestInfoFunction(
		t,
		GetFilesystemInfo,
		"/apis/subresources.kubevirt.io/v1/namespaces/default/virtualmachineinstances/test-vm/filesystemlist",
		createFilesystemListResponse,
		"filesystems",
		"failed to get filesystem info",
	)
}

func TestGetUserInfo(t *testing.T) {
	testGuestInfoFunction(
		t,
		GetUserInfo,
		"/apis/subresources.kubevirt.io/v1/namespaces/default/virtualmachineinstances/test-vm/userlist",
		createUserListResponse,
		"users",
		"failed to get user info",
	)
}

func TestGetNetworkInfo(t *testing.T) {
	testGuestInfoFunction(
		t,
		GetNetworkInfo,
		"/apis/subresources.kubevirt.io/v1/namespaces/default/virtualmachineinstances/test-vm/interfacelist",
		createInterfaceListResponse,
		"networkInterfaces",
		"failed to get network interface info",
	)
}

func TestGetAllGuestInfo(t *testing.T) {
	tests := []struct {
		name          string
		namespace     string
		vmName        string
		setupServer   func() *httptest.Server
		wantError     bool
		errorContains string
		wantKeys      []string
		wantErrorKeys []string
	}{
		{
			name:      "successfully retrieves all guest info",
			namespace: "default",
			vmName:    "test-vm",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					switch r.URL.Path {
					case "/apis/subresources.kubevirt.io/v1/namespaces/default/virtualmachineinstances/test-vm/guestosinfo":
						_ = json.NewEncoder(w).Encode(createGuestOSInfoResponse())
					case "/apis/subresources.kubevirt.io/v1/namespaces/default/virtualmachineinstances/test-vm/filesystemlist":
						_ = json.NewEncoder(w).Encode(createFilesystemListResponse())
					case "/apis/subresources.kubevirt.io/v1/namespaces/default/virtualmachineinstances/test-vm/userlist":
						_ = json.NewEncoder(w).Encode(createUserListResponse())
					case "/apis/subresources.kubevirt.io/v1/namespaces/default/virtualmachineinstances/test-vm/interfacelist":
						_ = json.NewEncoder(w).Encode(createInterfaceListResponse())
					default:
						http.NotFound(w, r)
					}
				}))
			},
			wantError: false,
			wantKeys:  []string{"guestOSInfo", "filesystems", "users", "networkInterfaces"},
		},
		{
			name:      "partial success - some queries fail",
			namespace: "default",
			vmName:    "test-vm",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					// Only guestosinfo succeeds
					if r.URL.Path == "/apis/subresources.kubevirt.io/v1/namespaces/default/virtualmachineinstances/test-vm/guestosinfo" {
						_ = json.NewEncoder(w).Encode(createGuestOSInfoResponse())
						return
					}
					http.NotFound(w, r)
				}))
			},
			wantError:     false,
			wantKeys:      []string{"guestOSInfo"},
			wantErrorKeys: []string{"filesystemsError", "usersError", "networkInterfacesError"},
		},
		{
			name:      "all queries fail",
			namespace: "default",
			vmName:    "test-vm",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.NotFound(w, r)
				}))
			},
			wantError:     true,
			errorContains: "guest agent is not responding",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			config := createTestRESTConfig(server)
			result, err := GetAllGuestInfo(context.Background(), config, tt.namespace, tt.vmName)

			if tt.wantError {
				if err == nil {
					t.Error("Expected error, got nil")
					return
				}
				if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("Error = %v, want to contain %q", err, tt.errorContains)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Error("Expected non-nil result, got nil")
				return
			}

			// Check expected data keys
			for _, key := range tt.wantKeys {
				if _, ok := result[key]; !ok {
					t.Errorf("Result missing expected key %q", key)
				}
			}

			// Check expected error keys
			for _, key := range tt.wantErrorKeys {
				if _, ok := result[key]; !ok {
					t.Errorf("Result missing expected error key %q", key)
				}
			}
		})
	}
}

func TestGetVMISubresource(t *testing.T) {
	tests := []struct {
		name        string
		namespace   string
		vmiName     string
		subresource string
		setupServer func() *httptest.Server
		wantError   bool
	}{
		{
			name:        "successfully retrieves subresource",
			namespace:   "default",
			vmiName:     "test-vm",
			subresource: "guestosinfo",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					response := &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "subresources.kubevirt.io/v1",
							"kind":       "VirtualMachineInstanceGuestOSInfo",
							"test":       "data",
						},
					}
					_ = json.NewEncoder(w).Encode(response)
				}))
			},
			wantError: false,
		},
		{
			name:        "returns error when subresource not found",
			namespace:   "default",
			vmiName:     "test-vm",
			subresource: "guestosinfo",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.NotFound(w, r)
				}))
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			config := createTestRESTConfig(server)
			result, err := getVMISubresource(context.Background(), config, tt.namespace, tt.vmiName, tt.subresource)

			if tt.wantError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Error("Expected non-nil result, got nil")
			}
		})
	}
}

// guestInfoTestFunc is a function type for guest info retrieval functions
type guestInfoTestFunc func(context.Context, *rest.Config, string, string) (map[string]any, error)

// testGuestInfoFunction is a generic test helper for guest info functions
func testGuestInfoFunction(
	t *testing.T,
	fn guestInfoTestFunc,
	urlPath string,
	responseFactory func() map[string]interface{},
	resultKey string,
	errorMessage string,
) {
	t.Helper()

	tests := []struct {
		name          string
		namespace     string
		vmName        string
		setupServer   func() *httptest.Server
		wantError     bool
		errorContains string
	}{
		{
			name:      "successfully retrieves info",
			namespace: "default",
			vmName:    "test-vm",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == urlPath {
						w.Header().Set("Content-Type", "application/json")
						_ = json.NewEncoder(w).Encode(responseFactory())
						return
					}
					http.NotFound(w, r)
				}))
			},
			wantError: false,
		},
		{
			name:      "returns error when guest agent not available",
			namespace: "default",
			vmName:    "test-vm",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.NotFound(w, r)
				}))
			},
			wantError:     true,
			errorContains: errorMessage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			config := createTestRESTConfig(server)
			result, err := fn(context.Background(), config, tt.namespace, tt.vmName)

			if tt.wantError {
				if err == nil {
					t.Error("Expected error, got nil")
					return
				}
				if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("Error = %v, want to contain %q", err, tt.errorContains)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Error("Expected non-nil result, got nil")
				return
			}

			if _, ok := result[resultKey]; !ok {
				t.Errorf("Result missing %q key", resultKey)
			}
		})
	}
}

// contains is a helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
