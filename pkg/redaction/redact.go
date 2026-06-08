package redaction

import (
	"fmt"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// RedactionModeOpaque replaces values with [REDACTED].
	RedactionModeOpaque = "opaque"
	// RedactionModeHashed replaces values with [REDACTED:gen_<id>:<hash>].
	RedactionModeHashed = "hashed"
)

// Redactor applies field-level redaction to Kubernetes resources based on configuration.
type Redactor struct {
	rules []redactionRule
	salt  *Salt
}

type redactionRule struct {
	group         string
	version       string
	kind          string
	fieldSegments []string // parsed dot-path segments e.g. ["data", "*"]
	redactionMode string
}

// NewRedactor creates a Redactor from the redacted resources configuration.
func NewRedactor(redactedResources []api.RedactedResource) *Redactor {
	var rules []redactionRule
	for _, r := range redactedResources {
		mode := r.Mode
		if mode == "" {
			mode = RedactionModeOpaque
		}
		for _, field := range r.Fields {
			rules = append(rules, redactionRule{
				group:         r.Group,
				version:       r.Version,
				kind:          r.Kind,
				fieldSegments: strings.Split(field, "."),
				redactionMode: mode,
			})
		}
	}
	return &Redactor{
		rules: rules,
		salt:  GlobalSalt(),
	}
}

// Apply redacts fields in the given unstructured object based on its GVK.
// The object is modified in place.
func (r *Redactor) Apply(obj *unstructured.Unstructured) {
	if obj == nil || len(r.rules) == 0 {
		return
	}
	gvk := obj.GroupVersionKind()
	for _, rule := range r.rules {
		if matchesGVK(rule, gvk) {
			r.walkValue(obj.Object, rule.fieldSegments, rule.redactionMode)
		}
	}
}

// ApplyToList redacts fields in all items of an unstructured list.
func (r *Redactor) ApplyToList(list *unstructured.UnstructuredList) {
	if list == nil || len(r.rules) == 0 {
		return
	}
	for i := range list.Items {
		r.Apply(&list.Items[i])
	}
}

// matchesGVK checks whether a redaction rule applies to the given GVK.
// Note: config validation (validateRedactedResources) ensures that entries with
// redacted_fields always have kind set, so the empty-kind branch below is only
// reachable for plain deny entries that have no field-level redaction.
func matchesGVK(rule redactionRule, gvk schema.GroupVersionKind) bool {
	if rule.group != gvk.Group || rule.version != gvk.Version {
		return false
	}
	if rule.kind == "" {
		return true
	}
	return rule.kind == gvk.Kind
}

// walkValue is the unified entry point that dispatches based on the runtime type of val.
// It handles maps, slices, and scalars at each level of the path.
func (r *Redactor) walkValue(val interface{}, segments []string, mode string) {
	switch v := val.(type) {
	case map[string]interface{}:
		r.walkMap(v, segments, mode)
	case []interface{}:
		r.walkSlice(v, segments, mode)
	}
}

// walkMap traverses a map along the given path segments.
// - Named segment: navigate to that key
// - "*" wildcard: iterate ALL keys in the map
func (r *Redactor) walkMap(obj map[string]interface{}, segments []string, mode string) {
	if len(segments) == 0 || obj == nil {
		return
	}

	segment := segments[0]
	remaining := segments[1:]

	if segment == "*" {
		// Wildcard on a map: iterate all keys
		for key, val := range obj {
			if len(remaining) == 0 {
				// Leaf: redact this value
				obj[key] = r.redactLeaf(val, mode)
			} else {
				// Non-leaf: recurse into the value
				r.walkValue(val, remaining, mode)
			}
		}
		return
	}

	// Named segment: navigate to specific key
	val, exists := obj[segment]
	if !exists {
		return
	}

	if len(remaining) == 0 {
		// Leaf: redact this field
		obj[segment] = r.redactLeaf(val, mode)
		return
	}

	// Non-leaf: recurse into the value (works for both maps and slices)
	r.walkValue(val, remaining, mode)
}

// walkSlice traverses a slice along the given path segments.
// - "*" wildcard: iterate ALL items in the slice
// - Named segment on a slice is invalid and skipped
func (r *Redactor) walkSlice(arr []interface{}, segments []string, mode string) {
	if len(segments) == 0 {
		return
	}

	segment := segments[0]
	remaining := segments[1:]

	if segment == "*" {
		// Wildcard on a slice: iterate all items
		for i, item := range arr {
			if len(remaining) == 0 {
				// Leaf: redact each item
				arr[i] = r.redactLeaf(item, mode)
			} else {
				// Non-leaf: recurse into each item
				r.walkValue(item, remaining, mode)
			}
		}
		return
	}

	// Named segment on a slice: not meaningful, skip
}

// redactLeaf replaces a value at a leaf position.
// For map values, it preserves the keys and redacts each inner value individually.
// For slice values, it redacts each element individually.
// For scalar values, it returns the redaction marker directly.
// If a leaf contains deeply nested structures (e.g. a map whose values are themselves
// maps), the inner values are collapsed to their fmt string representation and then
// redacted as scalars. This is intentional: leaf positions should not contain further
// traversable structure in typical Kubernetes resources.
func (r *Redactor) redactLeaf(val interface{}, mode string) interface{} {
	switch v := val.(type) {
	case map[string]interface{}:
		// Preserve keys, redact values
		redacted := make(map[string]interface{}, len(v))
		for key, innerVal := range v {
			redacted[key] = r.redactScalar(innerVal, mode)
		}
		return redacted
	case []interface{}:
		// Redact each element
		redacted := make([]interface{}, len(v))
		for i, item := range v {
			redacted[i] = r.redactScalar(item, mode)
		}
		return redacted
	default:
		return r.redactScalar(val, mode)
	}
}

// redactScalar replaces a single scalar value with the redaction marker.
func (r *Redactor) redactScalar(val interface{}, mode string) string {
	if mode == RedactionModeHashed {
		valStr := fmt.Sprintf("%v", val)
		return fmt.Sprintf("[REDACTED:gen_%s:%s]", r.salt.GenerationID(), r.salt.Hash(valStr))
	}
	return "[REDACTED]"
}
