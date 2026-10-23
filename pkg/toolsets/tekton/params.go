package tekton

import (
	"context"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// emptySchema is an empty JSON Schema (equivalent to JSON Schema `true`) that allows any additional properties.
var emptySchema = &jsonschema.Schema{}

const (
	pipelineKind    = "Pipeline"
	pipelineRunKind = "PipelineRun"
	taskKind        = "Task"
	taskRunKind     = "TaskRun"
)

// GroupVersionResource definitions for Tekton resources
var (
	pipelineGVR      = tektonv1.SchemeGroupVersion.WithResource("pipelines")
	pipelineRunGVR   = tektonv1.SchemeGroupVersion.WithResource("pipelineruns")
	taskGVR          = tektonv1.SchemeGroupVersion.WithResource("tasks")
	taskRunGVR       = tektonv1.SchemeGroupVersion.WithResource("taskruns")
	pacRepositoryGVR = schema.GroupVersionResource{
		Group:    "pipelinesascode.tekton.dev",
		Version:  "v1alpha1",
		Resource: "repositories",
	}
	tektonConfigGVR = schema.GroupVersionResource{
		Group:    "operator.tekton.dev",
		Version:  "v1alpha1",
		Resource: "tektonconfigs",
	}
)

func hasTektonKinds(ctx context.Context, inspector api.ClusterInspector, kinds ...string) func() bool {
	return func() bool {
		if inspector == nil {
			return true
		}
		hasKinds, err := inspector.Discovery().ServerResourcesForGroupVersion(ctx, tektonv1.SchemeGroupVersion.String()).Any(func(list *metav1.APIResourceList) bool {
			available := make(map[string]struct{}, len(list.APIResources))
			for _, resource := range list.APIResources {
				available[resource.Kind] = struct{}{}
			}
			for _, kind := range kinds {
				if _, ok := available[kind]; !ok {
					return false
				}
			}
			return true
		})
		return hasKinds || (err != nil && !api.IsNotFound(err))
	}
}

// parseParams converts a map[string]interface{} from a tool call argument into Tekton Params.
// Each map entry becomes a Param whose value type is inferred from the Go value:
//   - string  → ParamTypeString
//   - []interface{} → ParamTypeArray  (each element is coerced to string)
//   - map[string]interface{} → ParamTypeObject (each value is coerced to string)
func parseParams(raw map[string]interface{}) ([]tektonv1.Param, error) {
	params := make([]tektonv1.Param, 0, len(raw))
	for k, v := range raw {
		pv, err := toParamValue(k, v)
		if err != nil {
			return nil, err
		}
		params = append(params, tektonv1.Param{Name: k, Value: pv})
	}
	return params, nil
}

func toParamValue(name string, v interface{}) (tektonv1.ParamValue, error) {
	switch val := v.(type) {
	case string:
		return tektonv1.ParamValue{
			Type:      tektonv1.ParamTypeString,
			StringVal: val,
		}, nil

	case []interface{}:
		arr := make([]string, 0, len(val))
		for i, elem := range val {
			s, ok := elem.(string)
			if !ok {
				return tektonv1.ParamValue{}, fmt.Errorf("param %q: array element %d must be a string, got %T", name, i, elem)
			}
			arr = append(arr, s)
		}
		return tektonv1.ParamValue{
			Type:     tektonv1.ParamTypeArray,
			ArrayVal: arr,
		}, nil

	case map[string]interface{}:
		obj := make(map[string]string, len(val))
		for k, elem := range val {
			s, ok := elem.(string)
			if !ok {
				return tektonv1.ParamValue{}, fmt.Errorf("param %q: object key %q value must be a string, got %T", name, k, elem)
			}
			obj[k] = s
		}
		return tektonv1.ParamValue{
			Type:      tektonv1.ParamTypeObject,
			ObjectVal: obj,
		}, nil

	default:
		return tektonv1.ParamValue{}, fmt.Errorf("param %q: unsupported value type %T (expected string, array, or object)", name, v)
	}
}
