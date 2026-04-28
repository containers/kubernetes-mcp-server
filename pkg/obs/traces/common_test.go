package traces

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

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
