package output

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var podListJSON = `{
  "apiVersion": "v1", "kind": "PodList", "items": [{
    "apiVersion": "v1", "kind": "Pod",
    "metadata": {
      "name": "pod-1", "namespace": "default", "creationTimestamp": "2023-10-01T00:00:00Z", "labels": { "app": "nginx" }
    },
    "spec": { "containers": [{ "name": "container-1", "image": "marcnuri/chuck-norris" }] }
  }]
}`

type OutputSuite struct {
	suite.Suite
}

func (s *OutputSuite) podList() *unstructured.UnstructuredList {
	var podList unstructured.UnstructuredList
	s.Require().NoError(json.Unmarshal([]byte(podListJSON), &podList))
	return &podList
}

func (s *OutputSuite) TestTablePrintObj() {
	s.Run("processes the list", func() {
		_, err := Table.PrintObj(s.podList())
		s.NoError(err)
	})
	s.Run("prints headers", func() {
		out, _ := Table.PrintObj(s.podList())
		m, err := regexp.MatchString("NAME\\s+AGE\\s+LABELS", out)
		s.NoError(err)
		s.True(m, "Expected headers not found in output: %s", out)
	})
}

func (s *OutputSuite) TestTablePrintObjStructured() {
	s.Run("returns non-nil result", func() {
		result, err := Table.PrintObjStructured(s.podList())
		s.NoError(err)
		s.NotNil(result)
	})
	s.Run("text matches PrintObj output", func() {
		text, _ := Table.PrintObj(s.podList())
		result, _ := Table.PrintObjStructured(s.podList())
		s.Equal(text, result.Text)
	})
	s.Run("structured is nil for non-Table objects", func() {
		result, err := Table.PrintObjStructured(s.podList())
		s.NoError(err)
		// UnstructuredList is not a Table GVK, so structured will be nil
		// unless the API returns a Table response
		// The podList fixture is a PodList, not a Table
		s.Nil(result.Structured)
	})
}

func (s *OutputSuite) TestYamlPrintObjStructured() {
	s.Run("returns non-nil result", func() {
		result, err := Yaml.PrintObjStructured(s.podList())
		s.NoError(err)
		s.NotNil(result)
	})
	s.Run("text matches PrintObj output", func() {
		text, _ := Yaml.PrintObj(s.podList())
		result, _ := Yaml.PrintObjStructured(s.podList())
		s.Equal(text, result.Text)
	})
	s.Run("structured contains list items", func() {
		result, err := Yaml.PrintObjStructured(s.podList())
		s.NoError(err)
		items, ok := result.Structured.([]map[string]any)
		s.Require().True(ok, "expected []map[string]any, got %T", result.Structured)
		s.Len(items, 1)
	})
	s.Run("structured items have expected fields", func() {
		result, _ := Yaml.PrintObjStructured(s.podList())
		items := result.Structured.([]map[string]any)
		item := items[0]
		metadata, ok := item["metadata"].(map[string]any)
		s.Require().True(ok)
		s.Equal("pod-1", metadata["name"])
		s.Equal("default", metadata["namespace"])
	})
	s.Run("single object returns map", func() {
		var pod unstructured.Unstructured
		s.Require().NoError(json.Unmarshal([]byte(`{
			"apiVersion": "v1", "kind": "Pod",
			"metadata": { "name": "single-pod", "namespace": "test" }
		}`), &pod))
		result, err := Yaml.PrintObjStructured(&pod)
		s.NoError(err)
		obj, ok := result.Structured.(map[string]any)
		s.Require().True(ok, "expected map[string]any for single object")
		metadata, ok := obj["metadata"].(map[string]any)
		s.Require().True(ok)
		s.Equal("single-pod", metadata["name"])
	})
}

func (s *OutputSuite) TestTableToStructured() {
	s.Run("returns nil for nil table", func() {
		result := tableToStructured(nil)
		s.Nil(result)
	})
}

func TestOutput(t *testing.T) {
	suite.Run(t, new(OutputSuite))
}
