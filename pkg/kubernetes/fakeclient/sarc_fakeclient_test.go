package fakeclient

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

func TestCanIUse(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name         string
		deniedAccess []struct{ verb, group, resource, namespace, name string }

		gvr       schema.GroupVersionResource
		namespace string
		verb      string

		expectAllowed bool
	}{
		{
			name:         "allows all access by default",
			deniedAccess: nil,
			gvr: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "pods",
			},
			namespace:     "default",
			verb:          "list",
			expectAllowed: true,
		},
		{
			name: "denies access for any namespace pattern (verb:group:resource::) - core API",
			deniedAccess: []struct{ verb, group, resource, namespace, name string }{
				{verb: "list", group: "", resource: "pods", namespace: "", name: ""},
			},
			gvr: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "pods",
			},
			namespace:     "default",
			verb:          "list",
			expectAllowed: false,
		},
		{
			name: "allows access in different namespace when denied for specific namespace",
			deniedAccess: []struct{ verb, group, resource, namespace, name string }{
				{verb: "create", group: "", resource: "secrets", namespace: "restricted-ns", name: ""},
			},
			gvr: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "secrets",
			},
			namespace:     "default",
			verb:          "create",
			expectAllowed: true,
		},
		{
			name: "denies access for RBAC resources with API group",
			deniedAccess: []struct{ verb, group, resource, namespace, name string }{
				{verb: "create", group: "rbac.authorization.k8s.io", resource: "clusterrolebindings", namespace: "", name: ""},
			},
			gvr: schema.GroupVersionResource{
				Group:    "rbac.authorization.k8s.io",
				Version:  "v1",
				Resource: "clusterrolebindings",
			},
			namespace:     "",
			verb:          "create",
			expectAllowed: false,
		},
		{
			name: "allows different verb on same resource",
			deniedAccess: []struct{ verb, group, resource, namespace, name string }{
				{verb: "delete", group: "", resource: "pods", namespace: "", name: ""},
			},
			gvr: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "pods",
			},
			namespace:     "default",
			verb:          "list",
			expectAllowed: true,
		},
		{
			name: "denies access for apps API group resources in specific namespace",
			deniedAccess: []struct{ verb, group, resource, namespace, name string }{
				{verb: "patch", group: "apps", resource: "deployments", namespace: "production", name: ""},
			},
			gvr: schema.GroupVersionResource{
				Group:    "apps",
				Version:  "v1",
				Resource: "deployments",
			},
			namespace:     "production",
			verb:          "patch",
			expectAllowed: false,
		},
		{
			name: "allows access for same resource in different namespace",
			deniedAccess: []struct{ verb, group, resource, namespace, name string }{
				{verb: "patch", group: "apps", resource: "deployments", namespace: "production", name: ""},
			},
			gvr: schema.GroupVersionResource{
				Group:    "apps",
				Version:  "v1",
				Resource: "deployments",
			},
			namespace:     "staging",
			verb:          "patch",
			expectAllowed: true,
		},
		{
			name: "denies wildcard verb on custom resources",
			deniedAccess: []struct{ verb, group, resource, namespace, name string }{
				{verb: "*", group: "custom.example.com", resource: "widgets", namespace: "", name: ""},
			},
			gvr: schema.GroupVersionResource{
				Group:    "custom.example.com",
				Version:  "v1",
				Resource: "widgets",
			},
			namespace:     "default",
			verb:          "*",
			expectAllowed: false,
		},
		{
			name: "denies watch verb on configmaps in specific namespace",
			deniedAccess: []struct{ verb, group, resource, namespace, name string }{
				{verb: "watch", group: "", resource: "configmaps", namespace: "monitoring", name: ""},
			},
			gvr: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "configmaps",
			},
			namespace:     "monitoring",
			verb:          "watch",
			expectAllowed: false,
		},
		{
			name: "empty namespace defaults are handled correctly",
			deniedAccess: []struct{ verb, group, resource, namespace, name string }{
				{verb: "list", group: "", resource: "namespaces", namespace: "", name: ""},
			},
			gvr: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "namespaces",
			},
			namespace:     "",
			verb:          "list",
			expectAllowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build client options from denied access patterns
			opts := make([]Option, 0, len(tt.deniedAccess))
			for _, da := range tt.deniedAccess {
				opts = append(opts, WithDeniedAccess(da.verb, da.group, da.resource, da.namespace, da.name))
			}

			client := NewFakeSARCKubernetesClient(opts...)
			core := kubernetes.NewCore(client)

			allowed := core.CanIUse(ctx, &tt.gvr, tt.namespace, tt.verb)
			require.Equal(t, tt.expectAllowed, allowed)
		})
	}
}
