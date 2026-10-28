package traces

import (
	"encoding/base64"
	"encoding/hex"
)

// formatTrace flattens OTLP attributes in place to save tokens and converts
// base64 trace/span IDs to hex for readability.
func formatTrace(trace any) {
	root, _ := trace.(map[string]any)
	resourceSpans, _ := root["resourceSpans"].([]any)
	for _, resourceSpan := range resourceSpans {
		rs, _ := resourceSpan.(map[string]any)
		flattenAttributesIn(rs["resource"])

		scopeSpans, _ := rs["scopeSpans"].([]any)
		for _, scopeSpan := range scopeSpans {
			ss, _ := scopeSpan.(map[string]any)
			flattenAttributesIn(ss["scope"])

			spans, _ := ss["spans"].([]any)
			for _, s := range spans {
				span, _ := s.(map[string]any)
				hexifyID(span, "traceId")
				hexifyID(span, "spanId")
				hexifyID(span, "parentSpanId")
				flattenAttributesIn(span)

				events, _ := span["events"].([]any)
				for _, event := range events {
					flattenAttributesIn(event)
				}

				links, _ := span["links"].([]any)
				for _, link := range links {
					l, _ := link.(map[string]any)
					hexifyID(l, "traceId")
					hexifyID(l, "spanId")
					flattenAttributesIn(l)
				}
			}
		}
	}
}

// hexifyID rewrites the given trace/span ID field from the base64 encoding used by
// Tempo's JSON output to hex, so IDs match the hex form used in logs and other tools.
// Already-hex IDs and malformed values are left unchanged.
func hexifyID(obj map[string]any, key string) {
	s, ok := obj[key].(string)
	if !ok || s == "" {
		return
	}
	size := 8
	if key == "traceId" {
		size = 16
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b) != size {
		return
	}
	obj[key] = hex.EncodeToString(b)
}

// flattenAttributesIn replaces the OTLP KeyValue "attributes" list of obj with a
// plain key/value object.
func flattenAttributesIn(value any) {
	obj, _ := value.(map[string]any)
	list, ok := obj["attributes"].([]any)
	if !ok {
		return
	}
	obj["attributes"] = flattenAttributeList(list)
}

func flattenAttributeList(list []any) map[string]any {
	out := make(map[string]any, len(list))
	for _, item := range list {
		kv, _ := item.(map[string]any)
		key, ok := kv["key"].(string)
		if !ok {
			continue
		}
		out[key] = flattenAnyValue(kv["value"])
	}
	return out
}

// flattenAnyValue unwraps an OTLP AnyValue (e.g. {"stringValue": "x"}) into its
// underlying value. Nested array and kvlist values are flattened recursively.
func flattenAnyValue(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	for field, inner := range m {
		switch field {
		case "stringValue", "intValue", "boolValue", "doubleValue", "bytesValue":
			return inner
		case "arrayValue":
			av, _ := inner.(map[string]any)
			values, _ := av["values"].([]any)
			out := make([]any, len(values))
			for i, item := range values {
				out[i] = flattenAnyValue(item)
			}
			return out
		case "kvlistValue":
			kv, _ := inner.(map[string]any)
			values, _ := kv["values"].([]any)
			return flattenAttributeList(values)
		}
	}
	return v
}
