package output

import (
	"encoding/json"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"regexp"
	"strings"
	"testing"
)

func TestPlainTextUnstructuredList(t *testing.T) {
	var podList unstructured.UnstructuredList
	_ = json.Unmarshal([]byte(`
			{ "apiVersion": "v1", "kind": "PodList", "items": [{ 
			  "apiVersion": "v1", "kind": "Pod",
			  "metadata": {
			    "name": "pod-1", "namespace": "default", "creationTimestamp": "2023-10-01T00:00:00Z", "labels": { "app": "nginx" }
			  },
			  "spec": { "containers": [{ "name": "container-1", "image": "marcnuri/chuck-norris" }] } }
			]}`), &podList)
	out, err := Table.PrintObj(&podList)
	t.Run("processes the list", func(t *testing.T) {
		if err != nil {
			t.Fatalf("Error printing pod list: %v", err)
		}
	})
	t.Run("prints headers", func(t *testing.T) {
		expectedHeaders := "NAME\\s+AGE\\s+LABELS"
		if m, e := regexp.MatchString(expectedHeaders, out); !m || e != nil {
			t.Errorf("Expected headers '%s' not found in output: %s", expectedHeaders, out)
		}
	})
}

func TestJsonUnstructuredList(t *testing.T) {
	var podList unstructured.UnstructuredList
	_ = json.Unmarshal([]byte(`
			{ "apiVersion": "v1", "kind": "PodList", "items": [{ 
			  "apiVersion": "v1", "kind": "Pod",
			  "metadata": {
			    "name": "pod-1", "namespace": "default", "creationTimestamp": "2023-10-01T00:00:00Z", "labels": { "app": "nginx" }
			  },
			  "spec": { "containers": [{ "name": "container-1", "image": "marcnuri/chuck-norris" }] } }
			]}`), &podList)
	out, err := Json.PrintObj(&podList)
	t.Run("processes the list", func(t *testing.T) {
		if err != nil {
			t.Fatalf("Error printing pod list as JSON: %v", err)
		}
	})
	t.Run("outputs valid JSON", func(t *testing.T) {
		var result []map[string]any
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			t.Errorf("Output is not valid JSON: %v\nOutput: %s", err, out)
		}
	})
	t.Run("contains expected pod data", func(t *testing.T) {
		if !strings.Contains(out, `"name": "pod-1"`) {
			t.Errorf("Expected pod name 'pod-1' not found in JSON output: %s", out)
		}
		if !strings.Contains(out, `"namespace": "default"`) {
			t.Errorf("Expected namespace 'default' not found in JSON output: %s", out)
		}
		if !strings.Contains(out, `"image": "marcnuri/chuck-norris"`) {
			t.Errorf("Expected image 'marcnuri/chuck-norris' not found in JSON output: %s", out)
		}
	})
	t.Run("does not contain managedFields", func(t *testing.T) {
		if strings.Contains(out, "managedFields") {
			t.Errorf("JSON output should not contain managedFields: %s", out)
		}
	})
}

func TestJsonUnstructured(t *testing.T) {
	var pod unstructured.Unstructured
	_ = json.Unmarshal([]byte(`
			{ 
			  "apiVersion": "v1", "kind": "Pod",
			  "metadata": {
			    "name": "pod-1", "namespace": "default", "creationTimestamp": "2023-10-01T00:00:00Z", "labels": { "app": "nginx" }
			  },
			  "spec": { "containers": [{ "name": "container-1", "image": "marcnuri/chuck-norris" }] } 
			}`), &pod)
	out, err := Json.PrintObj(&pod)
	t.Run("processes single object", func(t *testing.T) {
		if err != nil {
			t.Fatalf("Error printing pod as JSON: %v", err)
		}
	})
	t.Run("outputs valid JSON", func(t *testing.T) {
		var result map[string]any
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			t.Errorf("Output is not valid JSON: %v\nOutput: %s", err, out)
		}
	})
	t.Run("contains expected pod data", func(t *testing.T) {
		if !strings.Contains(out, `"name": "pod-1"`) {
			t.Errorf("Expected pod name 'pod-1' not found in JSON output: %s", out)
		}
	})
}

func TestOutputFromString(t *testing.T) {
	t.Run("returns yaml output", func(t *testing.T) {
		out := FromString("yaml")
		if out == nil || out.GetName() != "yaml" {
			t.Errorf("Expected yaml output, got %v", out)
		}
	})
	t.Run("returns table output", func(t *testing.T) {
		out := FromString("table")
		if out == nil || out.GetName() != "table" {
			t.Errorf("Expected table output, got %v", out)
		}
	})
	t.Run("returns json output", func(t *testing.T) {
		out := FromString("json")
		if out == nil || out.GetName() != "json" {
			t.Errorf("Expected json output, got %v", out)
		}
	})
	t.Run("returns nil for unknown format", func(t *testing.T) {
		out := FromString("unknown")
		if out != nil {
			t.Errorf("Expected nil for unknown format, got %v", out)
		}
	})
}

func TestJsonAsTable(t *testing.T) {
	t.Run("json output does not use table format", func(t *testing.T) {
		if Json.AsTable() {
			t.Error("JSON output should not use table format")
		}
	})
}
