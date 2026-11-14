package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ParseManifestsSuite struct {
	suite.Suite
}

func names(s *ParseManifestsSuite, resource string) []string {
	objs, err := ParseManifests(resource)
	s.Require().NoError(err)
	ret := make([]string, 0, len(objs))
	for _, o := range objs {
		ret = append(ret, o.GetName())
	}
	return ret
}

const cmA = "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a\n"
const cmB = "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: b\n"
const cmC = "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: c\n"

func (s *ParseManifestsSuite) TestSeparators() {
	s.Run("plain separator", func() {
		s.Equal([]string{"a", "b"}, names(s, cmA+"---\n"+cmB))
	})
	s.Run("separator followed by a space keeps every later document", func() {
		s.Equal([]string{"a", "b", "c"}, names(s, cmA+"--- \n"+cmB+"---\n"+cmC))
	})
	s.Run("separator followed by a comment keeps every later document", func() {
		s.Equal([]string{"a", "b", "c"}, names(s, cmA+"--- # second\n"+cmB+"---\n"+cmC))
	})
	s.Run("CRLF line endings", func() {
		s.Equal([]string{"a", "b"}, names(s, "apiVersion: v1\r\nkind: ConfigMap\r\nmetadata:\r\n  name: a\r\n---\r\n"+cmB))
	})
	s.Run("leading separator", func() {
		s.Equal([]string{"a"}, names(s, "---\n"+cmA))
	})
	s.Run("trailing separator followed by a newline is ignored", func() {
		s.Equal([]string{"a", "b"}, names(s, cmA+"---\n"+cmB+"---\n"))
	})
	s.Run("trailing separator without a newline is ignored", func() {
		s.Equal([]string{"a", "b"}, names(s, cmA+"---\n"+cmB+"---"))
	})
	s.Run("comment-only document is skipped", func() {
		s.Equal([]string{"a", "c"}, names(s, cmA+"---\n# nothing here\n---\n"+cmC))
	})
	s.Run("blank document is skipped", func() {
		s.Equal([]string{"a", "c"}, names(s, cmA+"---\n\n---\n"+cmC))
	})
	s.Run("null document is skipped", func() {
		s.Equal([]string{"a", "c"}, names(s, cmA+"---\nnull\n---\n"+cmC))
	})
	s.Run("JSON object", func() {
		s.Equal([]string{"j"}, names(s, `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"j"}}`))
	})
	s.Run("integers decode as int64, as unstructured.Unstructured does", func() {
		objs, err := ParseManifests("apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: d\nspec:\n  replicas: 3\n")
		s.Require().NoError(err)
		s.IsType(int64(0), objs[0].Object["spec"].(map[string]interface{})["replicas"])
	})
	s.Run("status is removed", func() {
		objs, err := ParseManifests(cmA + "status:\n  phase: Ready\n")
		s.Require().NoError(err)
		_, hasStatus := objs[0].Object["status"]
		s.False(hasStatus)
	})
}

func (s *ParseManifestsSuite) TestErrors() {
	s.Run("only separators", func() {
		_, err := ParseManifests("---\n---\n")
		s.EqualError(err, "no Kubernetes object found in the resource argument")
	})
	s.Run("document without kind names its position", func() {
		_, err := ParseManifests(cmA + "---\napiVersion: v1\nmetadata:\n  name: b\n")
		s.EqualError(err, "document 2: apiVersion and kind are required (nothing was applied)")
	})
	s.Run("explicit empty object between documents is an error, as in kubectl", func() {
		objs, err := ParseManifests(cmA + "---\n{}\n---\n" + cmB)
		s.EqualError(err, "document 2: apiVersion and kind are required (nothing was applied)")
		s.Nil(objs)
	})
	s.Run("explicit empty object alone is an error", func() {
		_, err := ParseManifests("{}")
		s.EqualError(err, "document 1: apiVersion and kind are required (nothing was applied)")
	})
	s.Run("text after a separator is a syntax error, as in kubectl", func() {
		_, err := ParseManifests(cmA + "--- apiVersion: v1\n" + cmB)
		s.ErrorContains(err, "invalid Yaml document separator")
	})
	s.Run("invalid YAML names its position", func() {
		_, err := ParseManifests(cmA + "---\nkind: [unclosed\n")
		s.ErrorContains(err, "document 2:")
	})
}

func TestParseManifests(t *testing.T) {
	suite.Run(t, new(ParseManifestsSuite))
}
