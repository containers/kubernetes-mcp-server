package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
)

type CheckSuite struct {
	suite.Suite
}

func (s *CheckSuite) TestCheck() {
	dir := s.T().TempDir()
	manifest := `
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: ToolSet
metadata:
  name: sample
spec:
  description: A sample family
  prefix: sample
---
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: sample-namespaces
spec:
  toolset: sample
  description: List the namespaces of the cluster by name.
  readOnly: true
  examples:
    - summary: Every namespace
      arguments: {}
  inputSchema: {type: object}
  kubernetes: {verb: list, apiVersion: v1, kind: Namespace}
  output: response.items.map(n, n.metadata.name)
---
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: sample-pods
spec:
  toolset: sample
  description: Lists pods.
  readOnly: true
  inputSchema: {type: object, properties: {pod_name: {type: string}}}
  kubernetes: {verb: list, apiVersion: v1, kind: Pod}
---
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: sample-broken
spec:
  description: A tool whose output does not compile.
  kubernetes: {verb: list, apiVersion: v1, kind: Pod}
  output: "response.items.map(p, "
`
	s.Require().NoError(os.WriteFile(filepath.Join(dir, "tools.yaml"), []byte(manifest), 0o600))
	s.Run("one line per resource, greppable by prefix", func() {
		streams, out := testStream()
		cmd := NewMCPServer(streams)
		cmd.SetArgs([]string{"check", dir})
		err := cmd.Execute()
		s.ErrorContains(err, "check failed", "a failing resource fails the command")
		text := out.String()
		s.Contains(text, "PASS: "+filepath.Join(dir, "tools.yaml")+": ToolSet sample (dynamic/sample)")
		s.Contains(text, "PASS: "+filepath.Join(dir, "tools.yaml")+": Tool sample-namespaces (sample_namespaces)")
		s.Contains(text, "LINT: "+filepath.Join(dir, "tools.yaml")+": Tool sample-pods: argument pod_name is not camelCase")
		s.Contains(text, "LINT: "+filepath.Join(dir, "tools.yaml")+": Tool sample-pods: no examples")
		s.Contains(text, "FAIL: "+filepath.Join(dir, "tools.yaml")+": Tool sample-broken: ")
		s.Contains(text, "checked 4 resources: 2 pass, 1 with lints, 1 failed")
	})
	s.Run("standard input, and --strict fails on lints", func() {
		streams, out := testStream()
		streams.In = strings.NewReader(strings.SplitN(manifest, "---\napiVersion: tools.kubernetes-mcp-server.io/v1alpha1\nkind: Tool\nmetadata:\n  name: sample-broken", 2)[0])
		cmd := NewMCPServer(streams)
		cmd.SetArgs([]string{"check", "--strict", "-"})
		s.ErrorContains(cmd.Execute(), "check failed")
		s.Contains(out.String(), "LINT: stdin: Tool sample-pods")
		s.Contains(out.String(), "checked 3 resources: 2 pass, 1 with lints, 0 failed")
	})
	s.Run("a clean manifest passes", func() {
		streams, out := testStream()
		streams.In = strings.NewReader(strings.SplitN(manifest, "---\napiVersion: tools.kubernetes-mcp-server.io/v1alpha1\nkind: Tool\nmetadata:\n  name: sample-pods", 2)[0])
		cmd := NewMCPServer(streams)
		cmd.SetArgs([]string{"check", "--strict", "-"})
		s.NoError(cmd.Execute())
		s.Contains(out.String(), "checked 2 resources: 2 pass, 0 with lints, 0 failed")
	})
	s.Run("dashes that are not document separators do not split", func() {
		// A --- rule inside a description and a ---comment line must not
		// cut a document in two.
		dashed := `
apiVersion: tools.kubernetes-mcp-server.io/v1alpha1
kind: Tool
metadata:
  name: dashed
spec:
  description: |
    A description with a horizontal rule

    ----

    in the middle, and a separator look-alike below.
  readOnly: true
  examples:
    - summary: Nothing
      arguments: {}
  inputSchema: {type: object}
  kubernetes: {verb: list, apiVersion: v1, kind: Namespace}
  output: response.items.map(n, n.metadata.name)
--- # a trailing separator with a comment, as helm renders
`
		streams, out := testStream()
		streams.In = strings.NewReader(dashed)
		cmd := NewMCPServer(streams)
		cmd.SetArgs([]string{"check", "-"})
		s.NoError(cmd.Execute())
		s.Contains(out.String(), "PASS: stdin: Tool dashed (dashed)")
		s.Contains(out.String(), "checked 1 resources: 1 pass, 0 with lints, 0 failed")
	})
}

func TestCheck(t *testing.T) {
	suite.Run(t, new(CheckSuite))
}
