package tekton

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// emptySchema is an empty JSON Schema (equivalent to JSON Schema `true`) that allows any additional properties.
var emptySchema = &jsonschema.Schema{}

// GroupVersionResource definitions for Tekton resources
var (
	pipelineGVR = schema.GroupVersionResource{
		Group:    "tekton.dev",
		Version:  "v1",
		Resource: "pipelines",
	}
	pipelineRunGVR = schema.GroupVersionResource{
		Group:    "tekton.dev",
		Version:  "v1",
		Resource: "pipelineruns",
	}
	taskGVR = schema.GroupVersionResource{
		Group:    "tekton.dev",
		Version:  "v1",
		Resource: "tasks",
	}
	taskRunGVR = schema.GroupVersionResource{
		Group:    "tekton.dev",
		Version:  "v1",
		Resource: "taskruns",
	}
)

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
