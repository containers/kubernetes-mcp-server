package kubevirt

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateVMFromTemplate(t *testing.T) {
	tests := []struct {
		name          string
		namespace     string
		templateName  string
		parameters    map[string]string
		setupServer   func() *httptest.Server
		wantError     bool
		errorContains string
		wantKeys      []string
	}{
		{
			name:         "successfully creates VM from template",
			namespace:    "default",
			templateName: "test-template",
			parameters:   map[string]string{"VM_NAME": "my-vm"},
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					expectedPath := "/apis/subresources.template.kubevirt.io/v1beta1/namespaces/default/virtualmachinetemplates/test-template/create"
					if r.URL.Path != expectedPath {
						t.Errorf("unexpected path: got %s, want %s", r.URL.Path, expectedPath)
						http.NotFound(w, r)
						return
					}
					if r.Method != http.MethodPost {
						t.Errorf("unexpected method: got %s, want POST", r.Method)
						http.NotFound(w, r)
						return
					}

					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Errorf("failed to decode request body: %v", err)
						http.Error(w, "bad request", http.StatusBadRequest)
						return
					}
					if body["kind"] != "CreateOptions" {
						t.Errorf("unexpected kind: got %v, want CreateOptions", body["kind"])
					}

					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{
						"apiVersion": "kubevirt.io/v1",
						"kind":       "VirtualMachine",
						"metadata": map[string]any{
							"name":      "my-vm",
							"namespace": "default",
						},
						"spec": map[string]any{
							"runStrategy": "Halted",
						},
					})
				}))
			},
			wantError: false,
			wantKeys:  []string{"apiVersion", "kind", "metadata", "spec"},
		},
		{
			name:         "successfully creates VM without parameters",
			namespace:    "default",
			templateName: "test-template",
			parameters:   nil,
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						http.Error(w, "bad request", http.StatusBadRequest)
						return
					}
					if _, hasParams := body["parameters"]; hasParams {
						t.Error("expected no parameters in request body")
					}

					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{
						"apiVersion": "kubevirt.io/v1",
						"kind":       "VirtualMachine",
						"metadata": map[string]any{
							"name":      "generated-vm",
							"namespace": "default",
						},
					})
				}))
			},
			wantError: false,
			wantKeys:  []string{"apiVersion", "kind", "metadata"},
		},
		{
			name:         "returns error when template not found",
			namespace:    "default",
			templateName: "nonexistent",
			parameters:   nil,
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.NotFound(w, r)
				}))
			},
			wantError:     true,
			errorContains: "failed to create VM from template",
		},
		{
			name:         "returns error on server error",
			namespace:    "default",
			templateName: "test-template",
			parameters:   map[string]string{"VM_NAME": "my-vm"},
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, "internal server error", http.StatusInternalServerError)
				}))
			},
			wantError:     true,
			errorContains: "failed to create VM from template",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			config := createTestRESTConfig(server)
			result, err := CreateVMFromTemplate(context.Background(), config, tt.namespace, tt.templateName, tt.parameters)

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

			for _, key := range tt.wantKeys {
				if _, ok := result[key]; !ok {
					t.Errorf("Result missing expected key %q", key)
				}
			}
		})
	}
}
