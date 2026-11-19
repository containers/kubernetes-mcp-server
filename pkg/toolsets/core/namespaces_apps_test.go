package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/containers/kubernetes-mcp-server/pkg/output"
)

type NamespaceAppSuite struct{ suite.Suite }

func (s *NamespaceAppSuite) TestNamespaceAppStructured() {
	created := time.Now().Add(-26 * time.Hour).UTC().Format(time.RFC3339)
	list := &unstructured.UnstructuredList{Items: []unstructured.Unstructured{{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]any{
			"name":              "team-a",
			"creationTimestamp": created,
			"labels": map[string]any{
				"z": "last",
				"a": "first",
			},
		},
		"status": map[string]any{"phase": "Active"},
	}}}}

	s.Run("flattens a raw list into UI rows", func() {
		payload := tableAppStructured(list, nil)
		items, ok := payload["items"].([]map[string]any)
		s.Require().True(ok)
		s.Require().Len(items, 1)
		s.Equal(map[string]any{
			"Name":       "team-a",
			"Status":     "Active",
			"Age":        "1d",
			"Labels":     "a=first,z=last",
			"apiVersion": "v1",
			"kind":       "Namespace",
		}, items[0])
	})

	s.Run("preserves already-flat table rows", func() {
		rows := []map[string]any{{"Name": "team-a", "Status": "Active"}}
		payload := tableAppStructured(&unstructured.Unstructured{}, rows)
		s.Equal(rows, payload["items"])
	})

	s.Run("normalizes YAML rows before considering the structured slice", func() {
		printed, err := output.Yaml.PrintObjStructured(list)
		s.Require().NoError(err)
		payload := tableAppStructured(list, printed.Structured)
		rows := payload["items"].([]map[string]any)
		s.Equal("team-a", rows[0]["Name"])
		s.NotContains(rows[0], "metadata")
		s.NotContains(rows[0], "status")
	})
}

func TestNamespaceAppSuite(t *testing.T) {
	suite.Run(t, new(NamespaceAppSuite))
}
