package core

import (
	"context"
	"errors"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

type resourcesListTestRequest struct {
	args map[string]any
}

func (r resourcesListTestRequest) GetArguments() map[string]any {
	return r.args
}

type apiResourcesListDiscovery struct {
	discovery.DiscoveryInterface
	discovery.ServerResourcesInterfaceWithContext
	resources []*metav1.APIResourceList
	err       error
}

func (d apiResourcesListDiscovery) ServerPreferredResourcesWithContext(context.Context) ([]*metav1.APIResourceList, error) {
	return d.resources, d.err
}

type apiResourcesListKubernetesClient struct {
	api.KubernetesClient
	discovery discovery.DiscoveryInterface
}

func (c apiResourcesListKubernetesClient) RawDiscoveryClient() discovery.DiscoveryInterface {
	return c.discovery
}

func apiResourcesListParams(cfg *config.Config, client api.KubernetesClient, args map[string]any) api.ToolHandlerParams {
	return api.ToolHandlerParams{
		Context:          context.Background(),
		Config:           cfg,
		KubernetesClient: client,
		Request:          resourcesListTestRequest{args: args},
	}
}

func TestAPIResourcesListValidation(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{name: "missing kind", args: map[string]any{}, want: "kind parameter required"},
		{name: "wrong kind type", args: map[string]any{"kind": 123}, want: "kind parameter must be a string"},
		{name: "empty kind", args: map[string]any{"kind": "   "}, want: "kind parameter must not be empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := apiResourcesList(apiResourcesListParams(nil, nil, tt.args))
			if err != nil {
				t.Fatalf("apiResourcesList returned an unexpected Go error: %v", err)
			}
			if result == nil || result.Error == nil {
				t.Fatalf("apiResourcesList should return a structured error: %#v", result)
			}
			if !strings.Contains(result.Error.Error(), tt.want) {
				t.Fatalf("error %q does not contain %q", result.Error, tt.want)
			}
		})
	}
}

func TestAPIResourcesListRejectsNilConfig(t *testing.T) {
	result, err := apiResourcesList(apiResourcesListParams(nil, nil, map[string]any{"kind": "pod"}))
	if err != nil {
		t.Fatalf("apiResourcesList returned an unexpected Go error: %v", err)
	}
	if result == nil || result.Error == nil {
		t.Fatalf("apiResourcesList should return a structured error for nil config: %#v", result)
	}
	if !strings.Contains(result.Error.Error(), "config cannot be nil") {
		t.Fatalf("unexpected error: %v", result.Error)
	}
}

func TestAPIResourcesListDiscoveryResults(t *testing.T) {
	resources := []*metav1.APIResourceList{{
		GroupVersion: "v1",
		APIResources: []metav1.APIResource{{Name: "pods", Kind: "Pod", Namespaced: true}},
	}}

	tests := []struct {
		name         string
		discovered   []*metav1.APIResourceList
		discoveryErr error
		wantWarning  bool
		wantError    string
	}{
		{name: "success", discovered: resources},
		{name: "partial results", discovered: resources, discoveryErr: errors.New("one API group unavailable"), wantWarning: true},
		{name: "discovery failure", discoveryErr: errors.New("discovery failed"), wantError: "discovery failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := apiResourcesListDiscovery{resources: tt.discovered, err: tt.discoveryErr}
			client := apiResourcesListKubernetesClient{discovery: raw}
			result, err := apiResourcesList(apiResourcesListParams(config.New(), client, map[string]any{"kind": "pod"}))
			if err != nil {
				t.Fatalf("apiResourcesList returned an unexpected Go error: %v", err)
			}
			if result == nil {
				t.Fatal("apiResourcesList returned nil result")
			}
			if tt.wantError != "" {
				if result.Error == nil || !strings.Contains(result.Error.Error(), tt.wantError) {
					t.Fatalf("expected error containing %q, got %#v", tt.wantError, result.Error)
				}
				return
			}
			if result.Error != nil {
				t.Fatalf("apiResourcesList returned an unexpected tool error: %v", result.Error)
			}
			structured, ok := result.StructuredContent.(apiResourcesListResult)
			if !ok {
				t.Fatalf("unexpected structured result type: %T", result.StructuredContent)
			}
			if len(structured.Resources) != 1 || structured.Resources[0].Kind != "Pod" {
				t.Fatalf("unexpected resources: %#v", structured.Resources)
			}
			if tt.wantWarning && !strings.Contains(structured.Warning, "one API group unavailable") {
				t.Fatalf("expected partial discovery warning, got %q", structured.Warning)
			}
			if !tt.wantWarning && structured.Warning != "" {
				t.Fatalf("unexpected warning: %q", structured.Warning)
			}
		})
	}
}

func TestFilterDeniedAPIResources(t *testing.T) {
	resources := []kubernetes.APIResourceInfo{
		{APIVersion: "v1", Kind: "Pod", Name: "pods"},
		{APIVersion: "v1", Kind: "Secret", Name: "secrets"},
		{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Name: "roles"},
	}

	tests := []struct {
		name   string
		denied []config.GroupVersionKind
		want   []string
	}{
		{name: "no denials", want: []string{"Pod", "Secret", "Role"}},
		{
			name: "exact and group-wide denials",
			denied: []config.GroupVersionKind{
				{Version: "v1", Kind: "Secret"},
				{Group: "rbac.authorization.k8s.io", Version: "v1"},
			},
			want: []string{"Pod"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := filterDeniedAPIResources(context.Background(), resources, tt.denied)
			if len(filtered) != len(tt.want) {
				t.Fatalf("expected kinds %v, got %#v", tt.want, filtered)
			}
			for i, kind := range tt.want {
				if filtered[i].Kind != kind {
					t.Fatalf("expected kind %q at index %d, got %q", kind, i, filtered[i].Kind)
				}
			}
		})
	}
}
