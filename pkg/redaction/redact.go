package redaction

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	// RedactionModeOpaque replaces values with [REDACTED].
	RedactionModeOpaque = "opaque"
	// RedactionModeHashed replaces values with [REDACTED:gen_<id>:<hash>].
	RedactionModeHashed = "hashed"

	// lastAppliedConfigAnnotation is the kubectl annotation that stores the full
	// previous resource manifest, which would include unredacted Secret data.
	lastAppliedConfigAnnotation = "kubectl.kubernetes.io/last-applied-configuration"
)

// Redactor applies built-in Secret value redaction to Kubernetes resources.
// It redacts data.*, stringData.*, and strips the last-applied-configuration
// annotation from Secret resources regardless of API group or version.
type Redactor struct {
	mode string
	salt *Salt
}

// NewSecretRedactor creates a Redactor that redacts Secret values.
// The mode must be "opaque" or "hashed". If mode is empty, returns nil (no redaction).
func NewSecretRedactor(mode string) *Redactor {
	if mode == "" {
		return nil
	}
	return &Redactor{
		mode: mode,
		salt: GlobalSalt(),
	}
}

// Apply redacts Secret values in the given unstructured object.
// Non-Secret resources are left untouched.
// The object is modified in place.
func (r *Redactor) Apply(obj *unstructured.Unstructured) {
	if obj == nil || r == nil {
		return
	}
	// Match kind=Secret regardless of API group or version.
	// This avoids version-pinning bypasses (e.g. v1 vs v1beta1).
	if obj.GetKind() != "Secret" {
		return
	}
	r.redactSecretFields(obj)
}

// ApplyToList redacts Secret values in all items of an unstructured list.
func (r *Redactor) ApplyToList(list *unstructured.UnstructuredList) {
	if list == nil || r == nil {
		return
	}
	for i := range list.Items {
		r.Apply(&list.Items[i])
	}
}

// redactSecretFields redacts the data and stringData maps and strips the
// last-applied-configuration annotation which would contain unredacted values.
func (r *Redactor) redactSecretFields(obj *unstructured.Unstructured) {
	// Redact data.*
	r.redactMapField(obj.Object, "data")
	// Redact stringData.*
	r.redactMapField(obj.Object, "stringData")
	// Strip the last-applied-configuration annotation to prevent bypass
	annotations := obj.GetAnnotations()
	if annotations != nil {
		if _, exists := annotations[lastAppliedConfigAnnotation]; exists {
			delete(annotations, lastAppliedConfigAnnotation)
			if len(annotations) == 0 {
				// Remove the annotations key entirely if empty
				if md, ok := obj.Object["metadata"].(map[string]interface{}); ok {
					delete(md, "annotations")
				}
			} else {
				obj.SetAnnotations(annotations)
			}
		}
	}
}

// redactMapField redacts all values in a top-level map field of the object.
// Keys are preserved; values are replaced with the redaction marker.
func (r *Redactor) redactMapField(obj map[string]interface{}, field string) {
	val, exists := obj[field]
	if !exists {
		return
	}
	m, ok := val.(map[string]interface{})
	if !ok {
		return
	}
	for key, v := range m {
		m[key] = r.redactScalar(v)
	}
}

// redactScalar replaces a single value with the redaction marker.
func (r *Redactor) redactScalar(val interface{}) string {
	if r.mode == RedactionModeHashed {
		valStr := fmt.Sprintf("%v", val)
		return fmt.Sprintf("[REDACTED:gen_%s:%s]", r.salt.GenerationID(), r.salt.Hash(valStr))
	}
	return "[REDACTED]"
}
