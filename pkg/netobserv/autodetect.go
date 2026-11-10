package netobserv

import (
	"context"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// EffectiveConfig is the subset of FlowCollector configuration that affects the
// NetObserv MCP tool surface and its RBAC declaration.
type EffectiveConfig struct {
	Found             bool
	LokiEnabled       bool
	LokiMode          string
	PrometheusEnabled bool
	PrometheusMode    string
	Namespace         string
	Service           string
	Port              int
	Unknown           bool
}

var flowCollectorResources = map[schema.GroupVersionKind]schema.GroupVersionResource{
	{Group: "flows.netobserv.io", Version: "v1beta2", Kind: "FlowCollector"}: {Group: "flows.netobserv.io", Version: "v1beta2", Resource: "flowcollectors"},
	{Group: "flows.netobserv.io", Version: "v1beta1", Kind: "FlowCollector"}: {Group: "flows.netobserv.io", Version: "v1beta1", Resource: "flowcollectors"},
}

var flowCollectorGVKPreference = []schema.GroupVersionKind{
	{Group: "flows.netobserv.io", Version: "v1beta2", Kind: "FlowCollector"},
	{Group: "flows.netobserv.io", Version: "v1beta1", Kind: "FlowCollector"},
}

// DetectConfig reads the cluster-scoped FlowCollector named "cluster".
// A missing CR is a normal fallback to manual configuration; other errors are
// reported so callers can fail open rather than hiding tools.
func DetectConfig(ctx context.Context, inspector api.ClusterInspector) (EffectiveConfig, error) {
	if inspector == nil || inspector.Unstructured() == nil {
		return EffectiveConfig{}, nil
	}
	for _, gvk := range flowCollectorGVKPreference {
		obj, err := api.GetDefaultObject(ctx, inspector, flowCollectorResources[gvk], "cluster", metav1.GetOptions{})
		if err == nil {
			return effectiveConfigFromFlowCollector(obj)
		}
		if !api.IsNotFound(err) {
			return EffectiveConfig{}, fmt.Errorf("get FlowCollector cluster: %w", err)
		}
	}
	return EffectiveConfig{}, nil
}

func effectiveConfigFromFlowCollector(object *unstructured.Unstructured) (EffectiveConfig, error) {
	result := EffectiveConfig{Found: true, LokiEnabled: true, PrometheusEnabled: true}
	if enabled, ok, err := unstructured.NestedBool(object.Object, "spec", "loki", "enable"); err != nil {
		return EffectiveConfig{}, err
	} else if ok {
		result.LokiEnabled = enabled
	}
	if mode, ok, err := unstructured.NestedString(object.Object, "spec", "loki", "mode"); err != nil {
		return EffectiveConfig{}, err
	} else if ok {
		result.LokiMode = mode
	}
	if enabled, ok, err := unstructured.NestedBool(object.Object, "spec", "prometheus", "querier", "enable"); err != nil {
		return EffectiveConfig{}, err
	} else if ok {
		result.PrometheusEnabled = enabled
	}
	if mode, ok, err := unstructured.NestedString(object.Object, "spec", "prometheus", "querier", "mode"); err != nil {
		return EffectiveConfig{}, err
	} else if ok {
		result.PrometheusMode = mode
	}
	if namespace, ok, err := unstructured.NestedString(object.Object, "spec", "namespace"); err != nil {
		return EffectiveConfig{}, err
	} else if ok {
		result.Namespace = namespace
	}
	if port, ok, err := unstructured.NestedInt64(object.Object, "spec", "consolePlugin", "advanced", "port"); err != nil {
		return EffectiveConfig{}, err
	} else if ok {
		result.Port = int(port)
	}
	return result, nil
}
