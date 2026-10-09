package logs

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	logsdiscovery "github.com/rhobs/obs-mcp/pkg/logs/discovery"
)

var lokiStackGVRForTests = schema.GroupVersionResource{
	Group:    "loki.grafana.com",
	Version:  "v1",
	Resource: "lokistacks",
}

type mockGatewayResolver struct {
	gatewayURL string
}

var _ logsdiscovery.GatewayResolver = (*mockGatewayResolver)(nil)

func (m *mockGatewayResolver) ResolveGatewayURL(_ context.Context, _ dynamic.Interface, _, _, _ string) (string, error) {
	return m.gatewayURL, nil
}

// TestListInstancesHandler_NoResolver verifies the vanilla Kubernetes code path:
// when no resolver is set, plain HTTP service DNS is used regardless of tenants mode.
func TestListInstancesHandler_NoResolver(t *testing.T) {
	fakeClient := newMockLokiK8sClient(
		newLokiStack("openshift-logging", "logging-loki"),
	)

	result, err := listInstancesHandler(newTestParams(t, &Config{UseRoute: false}, fakeClient, nil))
	require.NoError(t, err)
	require.NoError(t, result.Error)
	output := result.StructuredContent.(ListInstancesOutput)
	require.Len(t, output.Instances, 1)
	require.Equal(t, "openshift-logging", output.Instances[0].LokiNamespace)
	require.Equal(t, "logging-loki", output.Instances[0].LokiName)
	require.Equal(t, "http://logging-loki-gateway-http.openshift-logging.svc:8080", output.Instances[0].URL)
}

// TestListInstancesHandler_WithResolver verifies that when an EndpointResolver is set,
// its ResolveGatewayURL result is used as the instance base URL.
func TestListInstancesHandler_WithResolver(t *testing.T) {
	fakeClient := newMockLokiK8sClient(
		newLokiStack("openshift-logging", "logging-loki"),
	)

	expected := "https://logging-loki-gateway-http.openshift-logging.svc:8080/api/logs/v1"
	resolver := &mockGatewayResolver{gatewayURL: expected}

	result, err := listInstancesHandler(newTestParams(t, &Config{Resolver: resolver}, fakeClient, nil))
	require.NoError(t, err)
	require.NoError(t, result.Error)
	output := result.StructuredContent.(ListInstancesOutput)
	require.Len(t, output.Instances, 1)
	require.Equal(t, "openshift-logging", output.Instances[0].LokiNamespace)
	require.Equal(t, "logging-loki", output.Instances[0].LokiName)
	require.Equal(t, expected, output.Instances[0].URL)
}

func newLokiStack(namespace, name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "loki.grafana.com",
		Version: "v1",
		Kind:    "LokiStack",
	})
	obj.SetNamespace(namespace)
	obj.SetName(name)
	obj.Object["spec"] = map[string]any{
		"tenants": map[string]any{
			"mode": "openshift-network",
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

func newMockLokiK8sClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		lokiStackGVRForTests: "LokiStackList",
	}, objects...)
}
