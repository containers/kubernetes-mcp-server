package mcp

import (
	"regexp"
	"slices"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

type NamespacesSuite struct {
	BaseMcpSuite
}

func (s *NamespacesSuite) TestNamespacesList() {
	s.InitMcpClient()
	s.Run("namespaces_list", func() {
		toolResult, err := s.CallTool("namespaces_list", map[string]interface{}{})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		s.Require().NotNil(toolResult, "Expected tool result from call")
		var decoded []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decoded)
		s.Run("has yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
		})
		s.Run("returns at least 3 items", func() {
			s.Truef(len(decoded) >= 3, "expected at least 3 items, got %v", len(decoded))
			for _, expectedNamespace := range []string{"default", "ns-1", "ns-2"} {
				s.Truef(slices.ContainsFunc(decoded, func(ns unstructured.Unstructured) bool {
					return ns.GetName() == expectedNamespace
				}), "namespace %s not found in the list", expectedNamespace)
			}
		})
	})
}

func (s *NamespacesSuite) TestNamespacesListDenied() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		denied_resources = [ { version = "v1", kind = "Namespace" } ]
	`), s.Cfg), "Expected to parse denied resources  config")
	s.InitMcpClient()
	s.Run("namespaces_list (denied)", func() {
		toolResult, err := s.CallTool("namespaces_list", map[string]interface{}{})
		s.Run("has error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			expectedMessage := "failed to list namespaces: resource not allowed: /v1, Kind=Namespace"
			s.Equalf(expectedMessage, toolResult.Content[0].(mcp.TextContent).Text,
				"expected descriptive error '%s', got %v", expectedMessage, toolResult.Content[0].(mcp.TextContent).Text)
		})
	})
}

func (s *NamespacesSuite) TestNamespacesListAsTable() {
	s.Cfg.ListOutput = "table"
	s.InitMcpClient()
	s.Run("namespaces_list (list_output=table)", func() {
		toolResult, err := s.CallTool("namespaces_list", map[string]interface{}{})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		s.Require().NotNil(toolResult, "Expected tool result from call")
		out := toolResult.Content[0].(mcp.TextContent).Text
		s.Run("returns column headers", func() {
			expectedHeaders := "APIVERSION\\s+KIND\\s+NAME\\s+STATUS\\s+AGE\\s+LABELS"
			m, e := regexp.MatchString(expectedHeaders, out)
			s.Truef(m, "Expected headers '%s' not found in output:\n%s", expectedHeaders, out)
			s.NoErrorf(e, "Error matching headers regex: %v", e)
		})
		s.Run("returns formatted row for ns-1", func() {
			expectedRow := "(?<apiVersion>v1)\\s+" +
				"(?<kind>Namespace)\\s+" +
				"(?<name>ns-1)\\s+" +
				"(?<status>Active)\\s+" +
				"(?<age>(\\d+m)?(\\d+s)?)\\s+" +
				"(?<labels>kubernetes.io/metadata.name=ns-1)"
			m, e := regexp.MatchString(expectedRow, out)
			s.Truef(m, "Expected row '%s' not found in output:\n%s", expectedRow, out)
			s.NoErrorf(e, "Error matching ns-1 regex: %v", e)
		})
		s.Run("returns formatted row for ns-2", func() {
			expectedRow := "(?<apiVersion>v1)\\s+" +
				"(?<kind>Namespace)\\s+" +
				"(?<name>ns-2)\\s+" +
				"(?<status>Active)\\s+" +
				"(?<age>(\\d+m)?(\\d+s)?)\\s+" +
				"(?<labels>kubernetes.io/metadata.name=ns-2)"
			m, e := regexp.MatchString(expectedRow, out)
			s.Truef(m, "Expected row '%s' not found in output:\n%s", expectedRow, out)
			s.NoErrorf(e, "Error matching ns-2 regex: %v", e)
		})
	})
}

func TestNamespaces(t *testing.T) {
	suite.Run(t, new(NamespacesSuite))
}

func TestProjectsListInOpenShift(t *testing.T) {
	testCaseWithContext(t, &mcpContext{before: inOpenShift, after: inOpenShiftClear}, func(c *mcpContext) {
		dynamicClient := dynamic.NewForConfigOrDie(envTestRestConfig)
		_, _ = dynamicClient.Resource(schema.GroupVersionResource{Group: "project.openshift.io", Version: "v1", Resource: "projects"}).
			Create(c.ctx, &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "project.openshift.io/v1",
				"kind":       "Project",
				"metadata": map[string]interface{}{
					"name": "an-openshift-project",
				},
			}}, metav1.CreateOptions{})
		toolResult, err := c.callTool("projects_list", map[string]interface{}{})
		t.Run("projects_list returns project list", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
			}
			if toolResult.IsError {
				t.Fatalf("call tool failed")
			}
		})
		var decoded []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decoded)
		t.Run("projects_list has yaml content", func(t *testing.T) {
			if err != nil {
				t.Fatalf("invalid tool result content %v", err)
			}
		})
		t.Run("projects_list returns at least 1 items", func(t *testing.T) {
			if len(decoded) < 1 {
				t.Errorf("invalid project count, expected at least 1, got %v", len(decoded))
			}
			idx := slices.IndexFunc(decoded, func(ns unstructured.Unstructured) bool {
				return ns.GetName() == "an-openshift-project"
			})
			if idx == -1 {
				t.Errorf("namespace %s not found in the list", "an-openshift-project")
			}
		})
	})
}

func TestProjectsListInOpenShiftDenied(t *testing.T) {
	deniedResourcesServer := test.Must(config.ReadToml([]byte(`
		denied_resources = [ { group = "project.openshift.io", version = "v1" } ]
	`)))
	testCaseWithContext(t, &mcpContext{staticConfig: deniedResourcesServer, before: inOpenShift, after: inOpenShiftClear}, func(c *mcpContext) {
		c.withEnvTest()
		projectsList, _ := c.callTool("projects_list", map[string]interface{}{})
		t.Run("projects_list has error", func(t *testing.T) {
			if !projectsList.IsError {
				t.Fatalf("call tool should fail")
			}
		})
		t.Run("projects_list describes denial", func(t *testing.T) {
			expectedMessage := "failed to list projects: resource not allowed: project.openshift.io/v1, Kind=Project"
			if projectsList.Content[0].(mcp.TextContent).Text != expectedMessage {
				t.Fatalf("expected descriptive error '%s', got %v", expectedMessage, projectsList.Content[0].(mcp.TextContent).Text)
			}
		})
	})
}
