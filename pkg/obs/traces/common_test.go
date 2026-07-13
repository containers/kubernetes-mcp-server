package traces

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/rest"
)

type mockKubernetesClient struct {
	api.KubernetesClient
	restConfig    *rest.Config
	dynamicClient *dynamicfake.FakeDynamicClient
}

func (m *mockKubernetesClient) RESTConfig() *rest.Config {
	return m.restConfig
}

func (m *mockKubernetesClient) DynamicClient() dynamic.Interface {
	return m.dynamicClient
}

type mockBaseConfig struct {
	api.BaseConfig
	config *Config
}

func (m *mockBaseConfig) GetToolsetConfig(name string) (api.ExtendedConfig, bool) {
	if name == ToolsetName && m.config != nil {
		return m.config, true
	}
	return nil, false
}

type mockToolCallRequest struct {
	arguments map[string]any
}

func (m *mockToolCallRequest) GetArguments() map[string]any {
	return m.arguments
}

func newTestParams(t *testing.T, cfg *Config, dynamicClient *dynamicfake.FakeDynamicClient, args map[string]any) api.ToolHandlerParams {
	t.Helper()
	return api.ToolHandlerParams{
		Context:          t.Context(),
		KubernetesClient: &mockKubernetesClient{restConfig: &rest.Config{}, dynamicClient: dynamicClient},
		BaseConfig:       &mockBaseConfig{config: cfg},
		ToolCallRequest:  &mockToolCallRequest{arguments: args},
	}
}

var (
	tempoStackGVR = schema.GroupVersionResource{
		Group:    "tempo.grafana.com",
		Version:  "v1alpha1",
		Resource: "tempostacks",
	}
	tempoMonolithicGVR = schema.GroupVersionResource{
		Group:    "tempo.grafana.com",
		Version:  "v1alpha1",
		Resource: "tempomonolithics",
	}
)

func newTempoStack(namespace, name string, tenants []string) *unstructured.Unstructured {
	auth := make([]any, 0, len(tenants))
	for _, t := range tenants {
		auth = append(auth, map[string]any{"tenantName": t})
	}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "tempo.grafana.com",
		Version: "v1alpha1",
		Kind:    "TempoStack",
	})
	obj.SetNamespace(namespace)
	obj.SetName(name)
	obj.Object["spec"] = map[string]any{
		"tenants": map[string]any{
			"mode":           "openshift",
			"authentication": auth,
		},
	}
	obj.Object["status"] = map[string]any{
		"conditions": []any{
			map[string]any{
				"type":   "Ready",
				"status": string(metav1.ConditionTrue),
			},
		},
	}
	return obj
}

func newTempoMonolithic(namespace, name string, tenants []string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "tempo.grafana.com",
		Version: "v1alpha1",
		Kind:    "TempoMonolithic",
	})
	obj.SetNamespace(namespace)
	obj.SetName(name)

	spec := map[string]any{}
	if len(tenants) > 0 {
		auth := make([]any, 0, len(tenants))
		for _, t := range tenants {
			auth = append(auth, map[string]any{"tenantName": t})
		}
		spec["multitenancy"] = map[string]any{
			"enabled":        true,
			"mode":           "openshift",
			"authentication": auth,
		}
	}
	obj.Object["spec"] = spec
	obj.Object["status"] = map[string]any{
		"conditions": []any{
			map[string]any{
				"type":   "Ready",
				"status": string(metav1.ConditionTrue),
			},
		},
	}
	return obj
}

func newMockK8sClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			tempoStackGVR:      "TempoStackList",
			tempoMonolithicGVR: "TempoMonolithicList",
		},
		objects...,
	)
}
