package mcp

import (
	"encoding/json"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type McpAppsSuite struct{ BaseMcpSuite }

func (s *McpAppsSuite) TestNamespacesListApp() {
	s.Cfg.AppsEnabled.SetForTest(true)
	s.InitMcpClient()

	s.Run("advertises the MCP Apps extension", func() {
		s.Require().NotNil(s.InitializeResult.Capabilities.Resources)
		s.Contains(s.InitializeResult.Capabilities.Extensions, "io.modelcontextprotocol/ui")
	})
	s.Run("associates namespaces_list with its UI resource", func() {
		tools, err := s.ListTools()
		s.Require().NoError(err)
		for _, tool := range tools.Tools {
			if tool.Name != "namespaces_list" {
				continue
			}
			ui, ok := tool.Meta["ui"].(map[string]any)
			s.Require().True(ok)
			s.Equal("ui://kubernetes-mcp-server/namespaces-list", ui["resourceUri"])
			return
		}
		s.Fail("namespaces_list was not registered")
	})
	s.Run("serves the application HTML through resources/read", func() {
		resource, err := s.Session.ReadResource(s.T().Context(), &gosdk.ReadResourceParams{
			URI: "ui://kubernetes-mcp-server/namespaces-list",
		})
		s.Require().NoError(err)
		s.Require().Len(resource.Contents, 1)
		s.Equal("text/html;profile=mcp-app", resource.Contents[0].MIMEType)
		s.Contains(resource.Contents[0].Text, "ui/notifications/tool-result")
		ui, ok := resource.Contents[0].Meta["ui"].(map[string]any)
		s.Require().True(ok)
		s.True(ui["prefersBorder"].(bool))
	})
	s.Run("returns flat UI rows independently from YAML output", func() {
		result, err := s.CallTool("namespaces_list", map[string]any{})
		s.Require().NoError(err)
		var payload struct {
			Items []map[string]any `json:"items"`
		}
		encoded, err := json.Marshal(result.StructuredContent)
		s.Require().NoError(err)
		s.Require().NoError(json.Unmarshal(encoded, &payload))
		for _, row := range payload.Items {
			s.NotContains(row, "metadata")
		}
	})
}

func (s *McpAppsSuite) TestAppsDisabled() {
	s.InitMcpClient()

	s.Run("does not associate namespaces_list with an app", func() {
		tools, err := s.ListTools()
		s.Require().NoError(err)
		for _, tool := range tools.Tools {
			if tool.Name == "namespaces_list" {
				s.NotContains(tool.Meta, "ui")
				return
			}
		}
		s.Fail("namespaces_list was not registered")
	})
	s.Run("does not register the app resource", func() {
		_, err := s.Session.ReadResource(s.T().Context(), &gosdk.ReadResourceParams{
			URI: "ui://kubernetes-mcp-server/namespaces-list",
		})
		s.Error(err)
	})
}

func (s *McpAppsSuite) TestAppsAdvertisedWithoutAnEnabledAppTool() {
	s.Cfg.AppsEnabled.SetForTest(true)
	s.Cfg.EnabledTools.SetForTest([]string{"pods_log"})
	s.InitMcpClient()

	s.Run("advertises the extension", func() {
		s.Contains(s.InitializeResult.Capabilities.Extensions, "io.modelcontextprotocol/ui")
	})
	s.Run("does not register an app resource", func() {
		_, err := s.Session.ReadResource(s.T().Context(), &gosdk.ReadResourceParams{
			URI: "ui://kubernetes-mcp-server/namespaces-list",
		})
		s.Error(err)
	})
}

func (s *McpAppsSuite) TestStandardAppsRegistration() {
	s.Cfg.AppsEnabled.SetForTest(true)
	s.Cfg.EnableTargetCompatibilityToolFilters.SetForTest(false)
	s.InitMcpClient()
	tools, err := s.ListTools()
	s.Require().NoError(err)
	expected := map[string]string{
		"namespaces_list": "table", "projects_list": "table", "pods_list": "table",
		"pods_list_in_namespace": "table", "resources_list": "table", "events_list": "table",
		"pods_top": "metrics", "nodes_top": "metrics", "pods_get": "resource", "resources_get": "resource",
	}
	for _, tool := range tools.Tools {
		kind, hasApp := expected[tool.Name]
		s.Run(tool.Name, func() {
			if !hasApp {
				s.NotContains(tool.Meta, "ui")
				return
			}
			ui, ok := tool.Meta["ui"].(map[string]any)
			s.Require().True(ok)
			uri, ok := ui["resourceUri"].(string)
			s.Require().True(ok)
			resource, err := s.Session.ReadResource(s.T().Context(), &gosdk.ReadResourceParams{URI: uri})
			s.Require().NoError(err)
			s.Require().Len(resource.Contents, 1)
			s.Equal("text/html;profile=mcp-app", resource.Contents[0].MIMEType)
			s.Contains(resource.Contents[0].Text, `data-app-kind="`+kind+`"`)
		})
		delete(expected, tool.Name)
	}
	s.Empty(expected, "expected every core app-bearing tool to be registered")
}

func (s *McpAppsSuite) TestStandardTableYamlResults()   { s.checkStandardTableResults("yaml") }
func (s *McpAppsSuite) TestStandardTableOutputResults() { s.checkStandardTableResults("table") }

func (s *McpAppsSuite) TestResourcesListYamlWithApps() {
	s.checkResourcesListCompatibility(true, "yaml")
}

func (s *McpAppsSuite) TestResourcesListYamlWithoutApps() {
	s.checkResourcesListCompatibility(false, "yaml")
}

func (s *McpAppsSuite) TestResourcesListTableWithoutApps() {
	s.checkResourcesListCompatibility(false, "table")
}

func (s *McpAppsSuite) TestEmptyResourceListWithApps() {
	s.checkEmptyResourceList(true, "table")
}

func (s *McpAppsSuite) TestEmptyResourceListYamlWithApps() {
	s.checkEmptyResourceList(true, "yaml")
}

func (s *McpAppsSuite) TestEmptyResourceListWithoutApps() {
	s.checkEmptyResourceList(false, "table")
}

func (s *McpAppsSuite) checkEmptyResourceList(appsEnabled bool, format string) {
	s.Cfg.AppsEnabled.SetForTest(appsEnabled)
	s.Cfg.ListOutput.SetForTest(format)
	s.InitMcpClient()
	result, err := s.CallTool("resources_list", map[string]any{
		"apiVersion": "v1", "kind": "Pod", "namespace": "ns-1", "labelSelector": "mcp-app-test=does-not-exist",
	})
	s.Require().NoError(err)
	s.Require().False(result.IsError, "%v", result.Content)
	if !appsEnabled {
		s.Nil(result.StructuredContent, "preserve the empty Table result when Apps is disabled")
		return
	}
	rows := s.appRows(result.StructuredContent)
	s.NotNil(rows, "empty results must be an array, not null")
	s.Empty(rows)
}

func (s *McpAppsSuite) checkResourcesListCompatibility(appsEnabled bool, format string) {
	s.Cfg.AppsEnabled.SetForTest(appsEnabled)
	s.Cfg.ListOutput.SetForTest(format)
	s.InitMcpClient()
	result, err := s.CallTool("resources_list", map[string]any{"apiVersion": "v1", "kind": "Pod", "namespace": "ns-1"})
	s.Require().NoError(err)
	s.Require().False(result.IsError, "%v", result.Content)
	rows := s.appRows(result.StructuredContent)
	s.Require().NotEmpty(rows)
	encoded, err := json.Marshal(result.StructuredContent)
	s.Require().NoError(err)
	var payload map[string]any
	s.Require().NoError(json.Unmarshal(encoded, &payload))
	s.NotContains(payload, "columns", "preserve the original structured result envelope")
	if format == "table" {
		s.NotEmpty(rows[0]["Name"])
		return
	}
	metadata, ok := rows[0]["metadata"].(map[string]any)
	s.Require().True(ok)
	s.Contains(rows[0], "spec")
	object, err := s.CallTool("resources_get", map[string]any{
		"apiVersion": "v1", "kind": "Pod", "namespace": "ns-1", "name": metadata["name"],
	})
	s.Require().NoError(err)
	s.Require().False(object.IsError, "%v", object.Content)
	expected, err := json.Marshal(object.StructuredContent)
	s.Require().NoError(err)
	actual, err := json.Marshal(rows[0])
	s.Require().NoError(err)
	s.JSONEq(string(expected), string(actual), "list rows must retain the complete resource object")
}

func (s *McpAppsSuite) checkStandardTableResults(format string) {
	s.Cfg.AppsEnabled.SetForTest(true)
	s.Cfg.ListOutput.SetForTest(format)
	s.InitMcpClient()
	for _, tool := range []struct {
		name string
		args map[string]any
	}{
		{"namespaces_list", map[string]any{}},
		{"pods_list", map[string]any{}},
		{"pods_list_in_namespace", map[string]any{"namespace": "ns-1"}},
		{"resources_list", map[string]any{"apiVersion": "v1", "kind": "Pod", "namespace": "ns-1"}},
	} {
		s.Run(tool.name, func() {
			result, err := s.CallTool(tool.name, tool.args)
			s.Require().NoError(err)
			s.Require().False(result.IsError, "%v", result.Content)
			rows := s.appRows(result.StructuredContent)
			s.Require().NotEmpty(rows)
			if tool.name == "resources_list" && format == "yaml" {
				return // Full objects are preserved; compatibility is tested separately.
			}
			for _, row := range rows {
				s.NotEmpty(row["Name"])
				for _, value := range row {
					_, nested := value.(map[string]any)
					s.False(nested, "table cells must be flat")
				}
			}
		})
	}
}

func (s *McpAppsSuite) TestStandardResourceResults() {
	s.Cfg.AppsEnabled.SetForTest(true)
	s.InitMcpClient()
	for _, tool := range []string{"pods_get", "resources_get"} {
		s.Run(tool, func() {
			args := map[string]any{"namespace": "ns-1", "name": "a-pod-in-ns-1"}
			if tool == "resources_get" {
				args["apiVersion"], args["kind"] = "v1", "Pod"
			}
			result, err := s.CallTool(tool, args)
			s.Require().NoError(err)
			s.Require().False(result.IsError, "%v", result.Content)
			encoded, err := json.Marshal(result.StructuredContent)
			s.Require().NoError(err)
			var object map[string]any
			s.Require().NoError(json.Unmarshal(encoded, &object))
			s.Equal("Pod", object["kind"])
			metadata, ok := object["metadata"].(map[string]any)
			s.Require().True(ok)
			s.Equal("a-pod-in-ns-1", metadata["name"])
			s.NotContains(metadata, "managedFields")
		})
	}
}

func (s *McpAppsSuite) TestStandardEventResults() {
	client := kubernetes.NewForConfigOrDie(test.EnvTestRestConfig())
	ns, err := client.CoreV1().Namespaces().Create(s.T().Context(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "mcp-app-events-"},
	}, metav1.CreateOptions{})
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = client.CoreV1().Namespaces().Delete(s.T().Context(), ns.Name, metav1.DeleteOptions{}) })
	s.Cfg.AppsEnabled.SetForTest(true)
	s.InitMcpClient()
	s.Run("empty event list is a structured empty array", func() {
		result, err := s.CallTool("events_list", map[string]any{"namespace": ns.Name})
		s.Require().NoError(err)
		s.Require().False(result.IsError)
		rows := s.appRows(result.StructuredContent)
		s.NotNil(rows)
		s.Empty(rows)
	})
	s.Run("event object references are flat while text remains YAML", func() {
		_, err := client.CoreV1().Events(ns.Name).Create(s.T().Context(), &corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{GenerateName: "app-event-"},
			InvolvedObject: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Name: "example", Namespace: ns.Name},
			Type:           "Normal", Reason: "Testing", Message: "Example event",
		}, metav1.CreateOptions{})
		s.Require().NoError(err)
		result, err := s.CallTool("events_list", map[string]any{"namespace": ns.Name})
		s.Require().NoError(err)
		s.Require().False(result.IsError)
		rows := s.appRows(result.StructuredContent)
		s.Require().Len(rows, 1)
		s.Equal("Pod/example", rows[0]["InvolvedObject"])
		s.Contains(result.Content[0].(*gosdk.TextContent).Text, "InvolvedObject:\n")
	})
}

func (s *McpAppsSuite) appRows(content any) []map[string]any {
	encoded, err := json.Marshal(content)
	s.Require().NoError(err)
	var payload struct {
		Items []map[string]any `json:"items"`
	}
	s.Require().NoError(json.Unmarshal(encoded, &payload))
	return payload.Items
}

func TestMcpApps(t *testing.T) {
	suite.Run(t, new(McpAppsSuite))
}
