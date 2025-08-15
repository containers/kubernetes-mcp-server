package olm

import (
	"testing"
)

func TestParseManifestYAML(t *testing.T) {
	manifest := `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
data:
  key: value
`
	u, err := parseManifest(manifest)
	if err != nil {
		t.Fatalf("parseManifest failed: %v", err)
	}
	if u.GetName() != "test-cm" {
		t.Fatalf("unexpected name: %s", u.GetName())
	}
	if u.GetKind() != "ConfigMap" {
		t.Fatalf("unexpected kind: %s", u.GetKind())
	}
	data, ok := u.Object["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data not found or wrong type")
	}
	if data["key"] != "value" {
		t.Fatalf("unexpected data value: %v", data["key"])
	}
}

func TestParseManifestJSON(t *testing.T) {
	manifest := `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"json-cm"},"data":{"a":"b"}}`
	u, err := parseManifest(manifest)
	if err != nil {
		t.Fatalf("parseManifest failed: %v", err)
	}
	if u.GetName() != "json-cm" {
		t.Fatalf("unexpected name: %s", u.GetName())
	}
	if u.GetKind() != "ConfigMap" {
		t.Fatalf("unexpected kind: %s", u.GetKind())
	}
}
