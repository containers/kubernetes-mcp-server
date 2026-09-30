package netobserv

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

var flowCollectorGVKs = []schema.GroupVersionKind{
	{Group: "flows.netobserv.io", Version: "v1beta2", Kind: "FlowCollector"},
	{Group: "flows.netobserv.io", Version: "v1beta1", Kind: "FlowCollector"},
}

var flowCollectorGVRs = []schema.GroupVersionResource{
	{Group: "flows.netobserv.io", Version: "v1beta2", Resource: "flowcollectors"},
	{Group: "flows.netobserv.io", Version: "v1beta1", Resource: "flowcollectors"},
}

// NetObservConfigProvider is implemented by cluster providers that can inspect
// the FlowCollector. It is intentionally optional so custom providers retain
// the existing manual configuration behavior.
type NetObservConfigProvider interface {
	NetObservConfig(context.Context) EffectiveConfig
}

// DetectConfig reads the cluster-scoped FlowCollector named "cluster".
// A missing CR is a normal fallback to manual configuration; other errors are
// reported so callers can fail open rather than hiding tools.
func DetectConfig(ctx context.Context, k8s api.KubernetesClient) (EffectiveConfig, error) {
	if k8s == nil || k8s.DiscoveryClient() == nil || k8s.DynamicClient() == nil {
		return EffectiveConfig{}, nil
	}
	var gvr schema.GroupVersionResource
	for i, gvk := range flowCollectorGVKs {
		has, err := api.HasGVKs(k8s.DiscoveryClient(), []schema.GroupVersionKind{gvk})
		if err != nil {
			return EffectiveConfig{}, err
		}
		if has {
			gvr = flowCollectorGVRs[i]
			break
		}
	}
	if gvr.Resource == "" {
		return EffectiveConfig{}, nil
	}
	obj, err := k8s.DynamicClient().Resource(gvr).Get(ctx, "cluster", metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return EffectiveConfig{}, nil
	}
	if err != nil {
		return EffectiveConfig{}, fmt.Errorf("get FlowCollector cluster: %w", err)
	}

	result, err := effectiveConfigFromFlowCollector(obj.Object)
	if err != nil {
		return EffectiveConfig{}, err
	}
	return result, nil
}

func effectiveConfigFromFlowCollector(object map[string]any) (EffectiveConfig, error) {
	result := EffectiveConfig{Found: true, LokiEnabled: true, PrometheusEnabled: true}
	if enabled, ok, err := nestedBool(object, "spec", "loki", "enable"); err != nil {
		return EffectiveConfig{}, err
	} else if ok {
		result.LokiEnabled = enabled
	}
	if mode, ok, err := nestedString(object, "spec", "loki", "mode"); err != nil {
		return EffectiveConfig{}, err
	} else if ok {
		result.LokiMode = mode
	}
	if enabled, ok, err := nestedBool(object, "spec", "prometheus", "querier", "enable"); err != nil {
		return EffectiveConfig{}, err
	} else if ok {
		result.PrometheusEnabled = enabled
	}
	if mode, ok, err := nestedString(object, "spec", "prometheus", "querier", "mode"); err != nil {
		return EffectiveConfig{}, err
	} else if ok {
		result.PrometheusMode = mode
	}
	if namespace, ok, err := nestedString(object, "spec", "namespace"); err != nil {
		return EffectiveConfig{}, err
	} else if ok {
		result.Namespace = namespace
	}
	if port, ok, err := nestedInt(object, "spec", "consolePlugin", "port"); err != nil {
		return EffectiveConfig{}, err
	} else if ok {
		result.Port = port
	}
	return result, nil
}

func nestedBool(obj map[string]any, fields ...string) (bool, bool, error) {
	value, ok, err := nestedValue(obj, fields...)
	if err != nil || !ok {
		return false, ok, err
	}
	b, ok := value.(bool)
	if !ok {
		return false, false, fmt.Errorf("%s must be boolean", strings.Join(fields, "."))
	}
	return b, true, nil
}

func nestedString(obj map[string]any, fields ...string) (string, bool, error) {
	value, ok, err := nestedValue(obj, fields...)
	if err != nil || !ok {
		return "", ok, err
	}
	s, ok := value.(string)
	if !ok {
		return "", false, fmt.Errorf("%s must be string", strings.Join(fields, "."))
	}
	return s, true, nil
}

func nestedInt(obj map[string]any, fields ...string) (int, bool, error) {
	value, ok, err := nestedValue(obj, fields...)
	if err != nil || !ok {
		return 0, ok, err
	}
	switch n := value.(type) {
	case int:
		return n, true, nil
	case int64:
		return int(n), true, nil
	case float64:
		return int(n), true, nil
	default:
		return 0, false, fmt.Errorf("%s must be integer", strings.Join(fields, "."))
	}
}

func nestedValue(obj map[string]any, fields ...string) (any, bool, error) {
	var current any = obj
	for _, field := range fields {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false, errors.New("invalid FlowCollector structure")
		}
		current, ok = m[field]
		if !ok {
			return nil, false, nil
		}
	}
	return current, true, nil
}
