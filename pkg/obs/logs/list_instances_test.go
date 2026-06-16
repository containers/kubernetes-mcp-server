package logs

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/rhobs/obs-mcp/pkg/logs/loki"
)

var lokiStackGVRForTests = schema.GroupVersionResource{
	Group:    "loki.grafana.com",
	Version:  "v1",
	Resource: "lokistacks",
}

func TestListInstancesHandler_Success(t *testing.T) {
	fakeClient := newMockLokiK8sClient(
		newLokiStack("openshift-logging", "logging-loki"),
	)

	output, err := ListInstancesHandler(ToolParams{
		context:       t.Context(),
		dynamicClient: fakeClient,
		config:        &Config{UseRoute: false},
		newLokiLoader: func(_, _ string) (loki.Loader, error) {
			return &mockLoader{}, nil
		},
	})
	require.NoError(t, err)
	require.Len(t, output.Instances, 1)
	require.Equal(t, "openshift-logging", output.Instances[0].LokiNamespace)
	require.Equal(t, "logging-loki", output.Instances[0].LokiName)
	require.Equal(t, "https://logging-loki-gateway-http.openshift-logging.svc:8080/api/logs/v1", output.Instances[0].URL)
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
