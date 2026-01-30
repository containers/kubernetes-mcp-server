package output

import (
	"encoding/json"
	"regexp"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
