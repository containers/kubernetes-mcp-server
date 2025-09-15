package mcp

import (
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/mark3labs/mcp-go/mcp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"regexp"
	"sigs.k8s.io/yaml"
	"slices"
	"testing"
)

func TestNamespacesList(t *testing.T) {
	testCase(t, func(c *mcpContext) {
		c.withEnvTest()
		toolResult, err := c.callTool("namespaces_list", map[string]interface{}{})
		t.Run("namespaces_list returns namespace list", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
			}
			if toolResult.IsError {
				t.Fatalf("call tool failed")
			}
		})
		var decoded []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decoded)
		t.Run("namespaces_list has yaml content", func(t *testing.T) {
			if err != nil {
				t.Fatalf("invalid tool result content %v", err)
			}
		})
		t.Run("namespaces_list returns at least 3 items", func(t *testing.T) {
			if len(decoded) < 3 {
				t.Errorf("invalid namespace count, expected at least 3, got %v", len(decoded))
			}
			for _, expectedNamespace := range []string{"default", "ns-1", "ns-2"} {
				idx := slices.IndexFunc(decoded, func(ns unstructured.Unstructured) bool {
					return ns.GetName() == expectedNamespace
				})
				if idx == -1 {
					t.Errorf("namespace %s not found in the list", expectedNamespace)
				}
			}
		})
	})
}

func TestNamespacesListDenied(t *testing.T) {
	deniedResourcesServer := &config.StaticConfig{DeniedResources: []config.GroupVersionKind{{Version: "v1", Kind: "Namespace"}}}
	testCaseWithContext(t, &mcpContext{staticConfig: deniedResourcesServer}, func(c *mcpContext) {
		c.withEnvTest()
		namespacesList, _ := c.callTool("namespaces_list", map[string]interface{}{})
		t.Run("namespaces_list has error", func(t *testing.T) {
			if !namespacesList.IsError {
				t.Fatalf("call tool should fail")
			}
		})
		t.Run("namespaces_list describes denial", func(t *testing.T) {
			expectedMessage := "failed to list namespaces: resource not allowed: /v1, Kind=Namespace"
			if namespacesList.Content[0].(mcp.TextContent).Text != expectedMessage {
				t.Fatalf("expected descriptive error '%s', got %v", expectedMessage, namespacesList.Content[0].(mcp.TextContent).Text)
			}
		})
	})
}

func TestNamespacesListAsTable(t *testing.T) {
	testCaseWithContext(t, &mcpContext{listOutput: output.Table}, func(c *mcpContext) {
		c.withEnvTest()
		toolResult, err := c.callTool("namespaces_list", map[string]interface{}{})
		t.Run("namespaces_list returns namespace list", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
			}
			if toolResult.IsError {
				t.Fatalf("call tool failed")
			}
		})
		out := toolResult.Content[0].(mcp.TextContent).Text
		t.Run("namespaces_list returns column headers", func(t *testing.T) {
			expectedHeaders := "APIVERSION\\s+KIND\\s+NAME\\s+STATUS\\s+AGE\\s+LABELS"
			if m, e := regexp.MatchString(expectedHeaders, out); !m || e != nil {
				t.Fatalf("Expected headers '%s' not found in output:\n%s", expectedHeaders, out)
			}
		})
		t.Run("namespaces_list returns formatted row for ns-1", func(t *testing.T) {
			expectedRow := "(?<apiVersion>v1)\\s+" +
				"(?<kind>Namespace)\\s+" +
				"(?<name>ns-1)\\s+" +
				"(?<status>Active)\\s+" +
				"(?<age>(\\d+m)?(\\d+s)?)\\s+" +
				"(?<labels>kubernetes.io/metadata.name=ns-1)"
			if m, e := regexp.MatchString(expectedRow, out); !m || e != nil {
				t.Fatalf("Expected row '%s' not found in output:\n%s", expectedRow, out)
			}
		})
		t.Run("namespaces_list returns formatted row for ns-2", func(t *testing.T) {
			expectedRow := "(?<apiVersion>v1)\\s+" +
				"(?<kind>Namespace)\\s+" +
				"(?<name>ns-2)\\s+" +
				"(?<status>Active)\\s+" +
				"(?<age>(\\d+m)?(\\d+s)?)\\s+" +
				"(?<labels>kubernetes.io/metadata.name=ns-2)"
			if m, e := regexp.MatchString(expectedRow, out); !m || e != nil {
				t.Fatalf("Expected row '%s' not found in output:\n%s", expectedRow, out)
			}
		})
	})

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
	deniedResourcesServer := &config.StaticConfig{DeniedResources: []config.GroupVersionKind{{Group: "project.openshift.io", Version: "v1"}}}
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
