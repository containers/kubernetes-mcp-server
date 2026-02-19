package mcp

import (
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"
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
	`), s.Cfg), "Expected to parse denied resources config")
	s.InitMcpClient()
	s.Run("namespaces_list (denied)", func() {
		toolResult, err := s.CallTool("namespaces_list", map[string]interface{}{})
		s.Run("has error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			msg := toolResult.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			expectedMessage := "failed to list namespaces:(.+:)? resource not allowed: /v1, Kind=Namespace"
			s.Regexpf(expectedMessage, msg,
				"expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
}

func (s *NamespacesSuite) TestNamespacesListForbidden() {
	s.InitMcpClient()
	defer restoreAuth(s.T().Context())
	client := kubernetes.NewForConfigOrDie(envTestRestConfig)
	// Remove all permissions - user will have forbidden access
	_ = client.RbacV1().ClusterRoles().Delete(s.T().Context(), "allow-all", metav1.DeleteOptions{})

	s.Run("namespaces_list (forbidden)", func() {
		capture := s.StartCapturingLogNotifications()
		toolResult, _ := s.CallTool("namespaces_list", map[string]interface{}{})
		s.Run("returns error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
			s.Contains(toolResult.Content[0].(mcp.TextContent).Text, "forbidden",
				"error message should indicate forbidden")
		})
		s.Run("sends log notification", func() {
			logNotification := capture.RequireLogNotification(s.T(), 2*time.Second)
			s.Equal("error", logNotification.Level, "forbidden errors should log at error level")
			s.Contains(logNotification.Data, "Permission denied", "log message should indicate permission denied")
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

func (s *NamespacesSuite) TestProjectsListInOpenShift() {
	s.Require().NoError(EnvTestInOpenShift(s.T().Context()), "Expected to configure test for OpenShift")
	s.T().Cleanup(func() {
		s.Require().NoError(EnvTestInOpenShiftClear(s.T().Context()), "Expected to clear OpenShift test configuration")
	})
	s.InitMcpClient()

	s.Run("projects_list returns project list in OpenShift", func() {
		dynamicClient := dynamic.NewForConfigOrDie(envTestRestConfig)
		_, _ = dynamicClient.Resource(schema.GroupVersionResource{Group: "project.openshift.io", Version: "v1", Resource: "projects"}).
			Create(s.T().Context(), &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "project.openshift.io/v1",
				"kind":       "Project",
				"metadata": map[string]interface{}{
					"name": "an-openshift-project",
				},
			}}, metav1.CreateOptions{})
		toolResult, err := s.CallTool("projects_list", map[string]interface{}{})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		var decoded []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decoded)
		s.Run("has yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
		})
		s.Run("returns at least 1 item", func() {
			s.GreaterOrEqualf(len(decoded), 1, "invalid project count, expected at least 1, got %v", len(decoded))
			idx := slices.IndexFunc(decoded, func(ns unstructured.Unstructured) bool {
				return ns.GetName() == "an-openshift-project"
			})
			s.NotEqualf(-1, idx, "namespace %s not found in the list", "an-openshift-project")
		})
	})
}

func (s *NamespacesSuite) TestProjectsListInOpenShiftDenied() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		denied_resources = [ { group = "project.openshift.io", version = "v1" } ]
	`), s.Cfg), "Expected to parse denied resources config")
	s.Require().NoError(EnvTestInOpenShift(s.T().Context()), "Expected to configure test for OpenShift")
	s.T().Cleanup(func() {
		s.Require().NoError(EnvTestInOpenShiftClear(s.T().Context()), "Expected to clear OpenShift test configuration")
	})
	s.InitMcpClient()

	s.Run("projects_list (denied)", func() {
		projectsList, err := s.CallTool("projects_list", map[string]interface{}{})
		s.Run("has error", func() {
			s.Truef(projectsList.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			msg := projectsList.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			expectedMessage := "failed to list projects:(.+:)? resource not allowed: project.openshift.io/v1, Kind=Project"
			s.Regexpf(expectedMessage, msg,
				"expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
}

func TestNamespaces(t *testing.T) {
	suite.Run(t, new(NamespacesSuite))
}
