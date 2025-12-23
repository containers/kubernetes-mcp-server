// Package test provides utilities for testing unstructured Kubernetes objects.
//
// The primary functionality is JSONPath-like field access for unstructured.Unstructured objects,
// making test assertions more readable and maintainable.
package test

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// FieldString retrieves a string field from an unstructured object using JSONPath-like notation.
// Returns the string value, or empty string if not found or not a string.
//
// IMPORTANT: This function cannot distinguish between "field doesn't exist", "field is nil",
// and "field exists with empty string value". When asserting empty string values (""),
// you should also verify the field exists using FieldExists:
//
//	s.True(test.FieldExists(obj, "spec.emptyField"), "field should exist")
//	s.Equal("", test.FieldString(obj, "spec.emptyField"), "field should be empty string")
//
// Examples:
//   - "spec.runStrategy"
//   - "spec.template.spec.volumes[0].containerDisk.image"
//   - "spec.dataVolumeTemplates[0].spec.sourceRef.kind"
func FieldString(obj *unstructured.Unstructured, path string) string {
	if obj == nil {
		return ""
	}
	value, _ := Field(obj.Object, path)
	if str, ok := value.(string); ok {
		return str
	}
	return ""
}

// FieldExists checks if a field exists at the given JSONPath-like path.
func FieldExists(obj *unstructured.Unstructured, path string) bool {
	if obj == nil {
		return false
	}
	_, found := Field(obj.Object, path)
	return found
}

// FieldInt retrieves an integer field from an unstructured object using JSONPath-like notation.
// Returns the integer value (int64), or 0 if not found or not an integer type (int, int64, int32).
//
// IMPORTANT: This function cannot distinguish between "field doesn't exist", "field is nil",
// and "field exists with value 0". When asserting zero values (0), you should also verify
// the field exists using FieldExists:
//
//	s.True(test.FieldExists(obj, "spec.zeroValue"), "field should exist")
//	s.Equal(int64(0), test.FieldInt(obj, "spec.zeroValue"), "field should be 0")
//
// Examples:
//   - "spec.replicas"
//   - "spec.ports[0].containerPort"
func FieldInt(obj *unstructured.Unstructured, path string) int64 {
	if obj == nil {
		return 0
	}
	value, _ := Field(obj.Object, path)
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case int32:
		return int64(v)
	default:
		return 0
	}
}

// FieldValue retrieves any field value from an unstructured object using JSONPath-like notation.
// Returns nil if the field is not found. This is useful when you need the raw value
// without type conversion.
// Examples:
//   - "spec.template.spec.containers[0]" - returns map[string]interface{}
//   - "metadata.labels" - returns map[string]interface{}
func FieldValue(obj *unstructured.Unstructured, path string) interface{} {
	if obj == nil {
		return nil
	}
	value, _ := Field(obj.Object, path)
	return value
}

// Field is the core helper that traverses an unstructured object using JSONPath-like notation.
// It supports both dot notation (foo.bar) and array indexing (foo[0].bar).
// Returns (nil, false) if any intermediate field is nil, as we cannot traverse through nil.
func Field(obj interface{}, path string) (interface{}, bool) {
	if obj == nil || path == "" {
		return nil, false
	}

	// Parse the path into segments
	segments := parsePath(path)
	current := obj

	for i, segment := range segments {
		if segment.isArray {
			// Handle array indexing
			slice, ok := current.([]interface{})
			if !ok {
				return nil, false
			}
			if segment.index >= len(slice) || segment.index < 0 {
				return nil, false
			}
			current = slice[segment.index]
		} else {
			// Handle map field access
			m, ok := current.(map[string]interface{})
			if !ok {
				return nil, false
			}
			val, exists := m[segment.field]
			if !exists {
				return nil, false
			}
			// If this is an intermediate field and value is nil, we can't traverse further
			if val == nil && i < len(segments)-1 {
				return nil, false
			}
			current = val
		}
	}

	return current, true
}

type pathSegment struct {
	field   string
	isArray bool
	index   int
}

// parsePath converts a JSONPath-like string into segments.
// Examples:
//   - "spec.runStrategy" -> [{field: "spec"}, {field: "runStrategy"}]
//   - "spec.volumes[0].name" -> [{field: "spec"}, {field: "volumes"}, {isArray: true, index: 0}, {field: "name"}]
func parsePath(path string) []pathSegment {
	var segments []pathSegment
	current := ""
	inBracket := false
	indexStr := ""

	for i := 0; i < len(path); i++ {
		ch := path[i]
		switch ch {
		case '.':
			if inBracket {
				indexStr += string(ch)
			} else if current != "" {
				segments = append(segments, pathSegment{field: current})
				current = ""
			}
		case '[':
			if current != "" {
				segments = append(segments, pathSegment{field: current})
				current = ""
			}
			inBracket = true
			indexStr = ""
		case ']':
			if inBracket {
				// Parse the index
				var idx int
				if _, err := fmt.Sscanf(indexStr, "%d", &idx); err != nil {
					// If parsing fails, use -1 as invalid index
					idx = -1
				}
				segments = append(segments, pathSegment{isArray: true, index: idx})
				inBracket = false
				indexStr = ""
			}
		default:
			if inBracket {
				indexStr += string(ch)
			} else {
				current += string(ch)
			}
		}
	}

	if current != "" {
		segments = append(segments, pathSegment{field: current})
	}

	return segments
}
