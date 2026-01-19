package mcp

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"
)

type ResourcesSuite struct {
	BaseMcpSuite
}

func (s *ResourcesSuite) TestResourcesList() {
	s.InitMcpClient()
	s.Run("resources_list with missing apiVersion returns error", func() {
		toolResult, _ := s.CallTool("resources_list", map[string]interface{}{})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf("failed to list resources, missing argument apiVersion", toolResult.Content[0].(mcp.TextContent).Text,
			"invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})
	s.Run("resources_list with missing kind returns error", func() {
		toolResult, _ := s.CallTool("resources_list", map[string]interface{}{"apiVersion": "v1"})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf("failed to list resources, missing argument kind", toolResult.Content[0].(mcp.TextContent).Text,
			"invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})
	s.Run("resources_list with invalid apiVersion returns error", func() {
		toolResult, _ := s.CallTool("resources_list", map[string]interface{}{"apiVersion": "invalid/api/version", "kind": "Pod"})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf("failed to list resources, invalid argument apiVersion", toolResult.Content[0].(mcp.TextContent).Text,
			"invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})
	s.Run("resources_list with nonexistent apiVersion returns error", func() {
		toolResult, _ := s.CallTool("resources_list", map[string]interface{}{"apiVersion": "custom.non.existent.example.com/v1", "kind": "Custom"})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf(`failed to list resources: no matches for kind "Custom" in version "custom.non.existent.example.com/v1"`,
			toolResult.Content[0].(mcp.TextContent).Text, "invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})
	s.Run("resources_list(apiVersion=v1, kind=Namespace) returns namespaces", func() {
		namespaces, err := s.CallTool("resources_list", map[string]interface{}{"apiVersion": "v1", "kind": "Namespace"})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(namespaces.IsError, "call tool failed")
		})
		var decodedNamespaces []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(namespaces.Content[0].(mcp.TextContent).Text), &decodedNamespaces)
		s.Run("has yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
		})
		s.Run("returns more than 2 items", func() {
			s.Truef(len(decodedNamespaces) >= 3, "invalid namespace count, expected >2, got %v", len(decodedNamespaces))
		})
	})
	s.Run("resources_list with label selector returns filtered pods", func() {
		s.Run("list pods with app=nginx label", func() {
			result, err := s.CallTool("resources_list", map[string]interface{}{
				"apiVersion":    "v1",
				"kind":          "Pod",
				"namespace":     "default",
				"labelSelector": "app=nginx",
			})
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(result.IsError, "call tool failed")

			var decodedPods []unstructured.Unstructured
			err = yaml.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &decodedPods)
			s.Nilf(err, "invalid tool result content %v", err)

			s.Lenf(decodedPods, 1, "expected 1 pod, got %d", len(decodedPods))
			s.Equalf("a-pod-in-default", decodedPods[0].GetName(), "expected a-pod-in-default, got %s", decodedPods[0].GetName())
		})
		s.Run("list pods with multiple label selectors", func() {
			result, err := s.CallTool("resources_list", map[string]interface{}{
				"apiVersion":    "v1",
				"kind":          "Pod",
				"namespace":     "default",
				"labelSelector": "test-label=test-value,another=value",
			})
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(result.IsError, "call tool failed")

			var decodedPods []unstructured.Unstructured
			err = yaml.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &decodedPods)
			s.Nilf(err, "invalid tool result content %v", err)

			s.Lenf(decodedPods, 0, "expected 0 pods, got %d", len(decodedPods))
		})
	})
	s.Run("resources_list with field selector returns filtered pods", func() {
		// Create an additional pod in default namespace to verify it gets excluded
		kc := kubernetes.NewForConfigOrDie(envTestRestConfig)
		_, _ = kc.CoreV1().Pods("default").Create(s.T().Context(), &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "resources-field-excluded",
				Labels: map[string]string{"app": "nginx"},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
		}, metav1.CreateOptions{})

		s.Run("list pods with metadata.name field selector returns only matching pod", func() {
			result, err := s.CallTool("resources_list", map[string]interface{}{
				"apiVersion":    "v1",
				"kind":          "Pod",
				"namespace":     "default",
				"fieldSelector": "metadata.name=a-pod-in-default",
			})
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(result.IsError, "call tool failed")

			var decodedPods []unstructured.Unstructured
			err = yaml.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &decodedPods)
			s.Nilf(err, "invalid tool result content %v", err)

			s.Lenf(decodedPods, 1, "expected exactly 1 pod, got %d", len(decodedPods))
			s.Equalf("a-pod-in-default", decodedPods[0].GetName(), "expected a-pod-in-default, got %s", decodedPods[0].GetName())
			for _, pod := range decodedPods {
				s.NotEqualf("resources-field-excluded", pod.GetName(), "resources-field-excluded should have been filtered out")
			}
		})
		s.Run("list pods with combined label and field selectors excludes pod with same label but different name", func() {
			result, err := s.CallTool("resources_list", map[string]interface{}{
				"apiVersion":    "v1",
				"kind":          "Pod",
				"namespace":     "default",
				"labelSelector": "app=nginx",
				"fieldSelector": "metadata.name=a-pod-in-default",
			})
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(result.IsError, "call tool failed")

			var decodedPods []unstructured.Unstructured
			err = yaml.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &decodedPods)
			s.Nilf(err, "invalid tool result content %v", err)

			s.Lenf(decodedPods, 1, "expected exactly 1 pod, got %d", len(decodedPods))
			s.Equalf("a-pod-in-default", decodedPods[0].GetName(), "expected a-pod-in-default, got %s", decodedPods[0].GetName())
			for _, pod := range decodedPods {
				s.NotEqualf("resources-field-excluded", pod.GetName(), "resources-field-excluded has same label but should be filtered by fieldSelector")
			}
		})
	})
}

func (s *ResourcesSuite) TestResourcesListDenied() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		denied_resources = [
			{ version = "v1", kind = "Secret" },
			{ group = "rbac.authorization.k8s.io", version = "v1" }
		]
	`), s.Cfg), "Expected to parse denied resources config")
	s.InitMcpClient()
	s.Run("resources_list (denied by kind)", func() {
		deniedByKind, err := s.CallTool("resources_list", map[string]interface{}{"apiVersion": "v1", "kind": "Secret"})
		s.Run("has error", func() {
			s.Truef(deniedByKind.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			msg := deniedByKind.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			expectedMessage := "failed to list resources:(.+:)? resource not allowed: /v1, Kind=Secret"
			s.Regexpf(expectedMessage, msg,
				"expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
	s.Run("resources_list (denied by group)", func() {
		deniedByGroup, err := s.CallTool("resources_list", map[string]interface{}{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role"})
		s.Run("has error", func() {
			s.Truef(deniedByGroup.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			msg := deniedByGroup.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			expectedMessage := "failed to list resources:(.+:)? resource not allowed: rbac.authorization.k8s.io/v1, Kind=Role"
			s.Regexpf(expectedMessage, msg,
				"expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
	s.Run("resources_list (not denied) returns list", func() {
		allowedResource, _ := s.CallTool("resources_list", map[string]interface{}{"apiVersion": "v1", "kind": "Namespace"})
		s.Falsef(allowedResource.IsError, "call tool should not fail")
	})
}

func (s *ResourcesSuite) TestResourcesListForbidden() {
	s.InitMcpClient()
	defer restoreAuth(s.T().Context())
	client := kubernetes.NewForConfigOrDie(envTestRestConfig)
	// Remove all permissions - user will have forbidden access
	_ = client.RbacV1().ClusterRoles().Delete(s.T().Context(), "allow-all", metav1.DeleteOptions{})

	s.Run("resources_list (forbidden)", func() {
		capture := s.StartCapturingLogNotifications()
		toolResult, _ := s.CallTool("resources_list", map[string]interface{}{"apiVersion": "v1", "kind": "ConfigMap"})
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

func (s *ResourcesSuite) TestResourcesListAsTable() {
	s.Cfg.ListOutput = "table"
	s.Require().NoError(EnvTestInOpenShift(s.T().Context()), "Expected to configure test for OpenShift")
	s.T().Cleanup(func() {
		s.Require().NoError(EnvTestInOpenShiftClear(s.T().Context()), "Expected to clear OpenShift test configuration")
	})
	s.InitMcpClient()

	s.Run("resources_list(apiVersion=v1, kind=ConfigMap) (list_output=table)", func() {
		kc := kubernetes.NewForConfigOrDie(envTestRestConfig)
		_, _ = kc.CoreV1().ConfigMaps("default").Create(s.T().Context(), &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "a-configmap-to-list-as-table", Labels: map[string]string{"resource": "config-map"}},
			Data:       map[string]string{"key": "value"},
		}, metav1.CreateOptions{})
		configMapList, err := s.CallTool("resources_list", map[string]interface{}{"apiVersion": "v1", "kind": "ConfigMap"})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(configMapList.IsError, "call tool failed")
		})
		s.Require().NotNil(configMapList, "Expected tool result from call")
		outConfigMapList := configMapList.Content[0].(mcp.TextContent).Text
		s.Run("returns column headers for ConfigMap list", func() {
			expectedHeaders := "NAMESPACE\\s+APIVERSION\\s+KIND\\s+NAME\\s+DATA\\s+AGE\\s+LABELS"
			m, e := regexp.MatchString(expectedHeaders, outConfigMapList)
			s.Truef(m, "Expected headers '%s' not found in output:\n%s", expectedHeaders, outConfigMapList)
			s.NoErrorf(e, "Error matching headers regex: %v", e)
		})
		s.Run("returns formatted row for a-configmap-to-list-as-table", func() {
			expectedRow := "(?<namespace>default)\\s+" +
				"(?<apiVersion>v1)\\s+" +
				"(?<kind>ConfigMap)\\s+" +
				"(?<name>a-configmap-to-list-as-table)\\s+" +
				"(?<data>1)\\s+" +
				"(?<age>(\\d+m)?(\\d+s)?)\\s+" +
				"(?<labels>resource=config-map)"
			m, e := regexp.MatchString(expectedRow, outConfigMapList)
			s.Truef(m, "Expected row '%s' not found in output:\n%s", expectedRow, outConfigMapList)
			s.NoErrorf(e, "Error matching row regex: %v", e)
		})
	})

	s.Run("resources_list(apiVersion=route.openshift.io/v1, kind=Route) (list_output=table)", func() {
		_, _ = dynamic.NewForConfigOrDie(envTestRestConfig).
			Resource(schema.GroupVersionResource{Group: "route.openshift.io", Version: "v1", Resource: "routes"}).
			Namespace("default").
			Create(s.T().Context(), &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "route.openshift.io/v1",
				"kind":       "Route",
				"metadata": map[string]interface{}{
					"name": "an-openshift-route-to-list-as-table",
				},
			}}, metav1.CreateOptions{})
		routeList, err := s.CallTool("resources_list", map[string]interface{}{"apiVersion": "route.openshift.io/v1", "kind": "Route"})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(routeList.IsError, "call tool failed")
		})
		s.Require().NotNil(routeList, "Expected tool result from call")
		outRouteList := routeList.Content[0].(mcp.TextContent).Text
		s.Run("returns column headers for Route list", func() {
			expectedHeaders := "NAMESPACE\\s+APIVERSION\\s+KIND\\s+NAME\\s+AGE\\s+LABELS"
			m, e := regexp.MatchString(expectedHeaders, outRouteList)
			s.Truef(m, "Expected headers '%s' not found in output:\n%s", expectedHeaders, outRouteList)
			s.NoErrorf(e, "Error matching headers regex: %v", e)
		})
		s.Run("returns formatted row for an-openshift-route-to-list-as-table", func() {
			expectedRow := "(?<namespace>default)\\s+" +
				"(?<apiVersion>route.openshift.io/v1)\\s+" +
				"(?<kind>Route)\\s+" +
				"(?<name>an-openshift-route-to-list-as-table)\\s+" +
				"(?<age>(\\d+m)?(\\d+s)?)\\s+" +
				"(?<labels><none>)"
			m, e := regexp.MatchString(expectedRow, outRouteList)
			s.Truef(m, "Expected row '%s' not found in output:\n%s", expectedRow, outRouteList)
			s.NoErrorf(e, "Error matching row regex: %v", e)
		})
	})
}

func (s *ResourcesSuite) TestResourcesGet() {
	s.InitMcpClient()
	s.Run("resources_get with missing apiVersion returns error", func() {
		toolResult, _ := s.CallTool("resources_get", map[string]interface{}{})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf("failed to get resource, missing argument apiVersion", toolResult.Content[0].(mcp.TextContent).Text,
			"invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})
	s.Run("resources_get with missing kind returns error", func() {
		toolResult, _ := s.CallTool("resources_get", map[string]interface{}{"apiVersion": "v1"})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf("failed to get resource, missing argument kind", toolResult.Content[0].(mcp.TextContent).Text,
			"invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})
	s.Run("resources_get with invalid apiVersion returns error", func() {
		toolResult, _ := s.CallTool("resources_get", map[string]interface{}{"apiVersion": "invalid/api/version", "kind": "Pod", "name": "a-pod"})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf("failed to get resource, invalid argument apiVersion", toolResult.Content[0].(mcp.TextContent).Text,
			"invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})
	s.Run("resources_get with nonexistent apiVersion returns error", func() {
		toolResult, _ := s.CallTool("resources_get", map[string]interface{}{"apiVersion": "custom.non.existent.example.com/v1", "kind": "Custom", "name": "a-custom"})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf(`failed to get resource: no matches for kind "Custom" in version "custom.non.existent.example.com/v1"`,
			toolResult.Content[0].(mcp.TextContent).Text, "invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})
	s.Run("resources_get with missing name returns error", func() {
		toolResult, _ := s.CallTool("resources_get", map[string]interface{}{"apiVersion": "v1", "kind": "Namespace"})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf("failed to get resource, missing argument name", toolResult.Content[0].(mcp.TextContent).Text,
			"invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})
	s.Run("resources_get with nonexistent resource", func() {
		capture := s.StartCapturingLogNotifications()
		toolResult, _ := s.CallTool("resources_get", map[string]interface{}{"apiVersion": "v1", "kind": "ConfigMap", "name": "nonexistent-configmap"})
		s.Run("returns error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
			s.Equalf(`failed to get resource: configmaps "nonexistent-configmap" not found`,
				toolResult.Content[0].(mcp.TextContent).Text, "invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
		})
		s.Run("sends log notification", func() {
			logNotification := capture.RequireLogNotification(s.T(), 2*time.Second)
			s.Equal("info", logNotification.Level, "not found errors should log at info level")
			s.Contains(logNotification.Data, "Resource not found", "log message should indicate resource not found")
		})
	})
	s.Run("resources_get returns namespace", func() {
		namespace, err := s.CallTool("resources_get", map[string]interface{}{"apiVersion": "v1", "kind": "Namespace", "name": "default"})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(namespace.IsError, "call tool failed")
		})
		var decodedNamespace unstructured.Unstructured
		err = yaml.Unmarshal([]byte(namespace.Content[0].(mcp.TextContent).Text), &decodedNamespace)
		s.Run("has yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
		})
		s.Run("returns default namespace", func() {
			s.Equalf("default", decodedNamespace.GetName(), "invalid namespace name, expected default, got %v", decodedNamespace.GetName())
		})
	})
}

func (s *ResourcesSuite) TestResourcesGetDenied() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		denied_resources = [
			{ version = "v1", kind = "Secret" },
			{ group = "rbac.authorization.k8s.io", version = "v1" }
		]
	`), s.Cfg), "Expected to parse denied resources config")
	s.InitMcpClient()
	kc := kubernetes.NewForConfigOrDie(envTestRestConfig)
	_, _ = kc.CoreV1().Secrets("default").Create(s.T().Context(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "denied-secret"},
	}, metav1.CreateOptions{})
	_, _ = kc.RbacV1().Roles("default").Create(s.T().Context(), &v1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "denied-role"},
	}, metav1.CreateOptions{})
	s.Run("resources_get (denied by kind)", func() {
		deniedByKind, err := s.CallTool("resources_get", map[string]interface{}{"apiVersion": "v1", "kind": "Secret", "namespace": "default", "name": "denied-secret"})
		s.Run("has error", func() {
			s.Truef(deniedByKind.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			msg := deniedByKind.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			expectedMessage := "failed to get resource:(.+:)? resource not allowed: /v1, Kind=Secret"
			s.Regexpf(expectedMessage, msg,
				"expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
	s.Run("resources_get (denied by group)", func() {
		deniedByGroup, err := s.CallTool("resources_get", map[string]interface{}{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role", "namespace": "default", "name": "denied-role"})
		s.Run("has error", func() {
			s.Truef(deniedByGroup.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			msg := deniedByGroup.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			expectedMessage := "failed to get resource:(.+:)? resource not allowed: rbac.authorization.k8s.io/v1, Kind=Role"
			s.Regexpf(expectedMessage, msg,
				"expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
	s.Run("resources_get (not denied) returns resource", func() {
		allowedResource, err := s.CallTool("resources_get", map[string]interface{}{"apiVersion": "v1", "kind": "Namespace", "name": "default"})
		s.Falsef(allowedResource.IsError, "call tool should not fail")
		s.Nilf(err, "call tool should not return error object")
	})
}

func (s *ResourcesSuite) TestResourcesCreateOrUpdate() {
	s.InitMcpClient()
	client := kubernetes.NewForConfigOrDie(envTestRestConfig)

	s.Run("resources_create_or_update with nil resource returns error", func() {
		toolResult, _ := s.CallTool("resources_create_or_update", map[string]interface{}{})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf("failed to create or update resources, missing argument resource", toolResult.Content[0].(mcp.TextContent).Text,
			"invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})
	s.Run("resources_create_or_update with empty resource returns error", func() {
		toolResult, _ := s.CallTool("resources_create_or_update", map[string]interface{}{"resource": ""})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf("failed to create or update resources, missing argument resource", toolResult.Content[0].(mcp.TextContent).Text,
			"invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})

	s.Run("resources_create_or_update with valid namespaced yaml resource", func() {
		configMapYaml := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a-cm-created-or-updated\n  namespace: default\n"
		resourcesCreateOrUpdateCm1, err := s.CallTool("resources_create_or_update", map[string]interface{}{"resource": configMapYaml})
		s.Run("returns success", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(resourcesCreateOrUpdateCm1.IsError, "call tool failed")
		})
		var decodedCreateOrUpdateCm1 []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(resourcesCreateOrUpdateCm1.Content[0].(mcp.TextContent).Text), &decodedCreateOrUpdateCm1)
		s.Run("returns yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
			s.Truef(strings.HasPrefix(resourcesCreateOrUpdateCm1.Content[0].(mcp.TextContent).Text, "# The following resources (YAML) have been created or updated successfully"),
				"Expected success message, got %v", resourcesCreateOrUpdateCm1.Content[0].(mcp.TextContent).Text)
			s.Lenf(decodedCreateOrUpdateCm1, 1, "invalid resource count, expected 1, got %v", len(decodedCreateOrUpdateCm1))
			s.Equalf("a-cm-created-or-updated", decodedCreateOrUpdateCm1[0].GetName(),
				"invalid resource name, expected a-cm-created-or-updated, got %v", decodedCreateOrUpdateCm1[0].GetName())
			s.NotEmptyf(decodedCreateOrUpdateCm1[0].GetUID(), "invalid uid, got %v", decodedCreateOrUpdateCm1[0].GetUID())
		})
		s.Run("creates ConfigMap", func() {
			cm, _ := client.CoreV1().ConfigMaps("default").Get(s.T().Context(), "a-cm-created-or-updated", metav1.GetOptions{})
			s.NotNil(cm, "ConfigMap not found")
		})
	})

	s.Run("resources_create_or_update with valid namespaced json resource", func() {
		configMapJson := "{\"apiVersion\": \"v1\", \"kind\": \"ConfigMap\", \"metadata\": {\"name\": \"a-cm-created-or-updated-2\", \"namespace\": \"default\"}}"
		resourcesCreateOrUpdateCm2, err := s.CallTool("resources_create_or_update", map[string]interface{}{"resource": configMapJson})
		s.Run("returns success", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(resourcesCreateOrUpdateCm2.IsError, "call tool failed")
		})
		s.Run("creates config map", func() {
			cm, _ := client.CoreV1().ConfigMaps("default").Get(s.T().Context(), "a-cm-created-or-updated-2", metav1.GetOptions{})
			s.NotNil(cm, "ConfigMap not found")
		})
	})

	s.Run("resources_create_or_update with valid cluster-scoped json resource", func() {
		customResourceDefinitionJson := `
          {
            "apiVersion": "apiextensions.k8s.io/v1",
            "kind": "CustomResourceDefinition",
            "metadata": {"name": "customs.example.com"},
            "spec": {
              "group": "example.com",
              "versions": [{
                "name": "v1","served": true,"storage": true,
                "schema": {"openAPIV3Schema": {"type": "object"}}
              }],
              "scope": "Namespaced",
              "names": {"plural": "customs","singular": "custom","kind": "Custom"}
            }
          }`
		resourcesCreateOrUpdateCrd, err := s.CallTool("resources_create_or_update", map[string]interface{}{"resource": customResourceDefinitionJson})
		s.Run("returns success", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(resourcesCreateOrUpdateCrd.IsError, "call tool failed")
		})
		s.Run("creates custom resource definition", func() {
			apiExtensionsV1Client := apiextensionsv1.NewForConfigOrDie(envTestRestConfig)
			_, err = apiExtensionsV1Client.CustomResourceDefinitions().Get(s.T().Context(), "customs.example.com", metav1.GetOptions{})
			s.Nilf(err, "custom resource definition not found")
		})
		s.Require().NoError(EnvTestWaitForAPIResourceCondition(s.T().Context(), "example.com", "v1", "customs", true))
	})

	s.Run("resources_create_or_update creates custom resource", func() {
		customJson := "{\"apiVersion\": \"example.com/v1\", \"kind\": \"Custom\", \"metadata\": {\"name\": \"a-custom-resource\"}}"
		resourcesCreateOrUpdateCustom, err := s.CallTool("resources_create_or_update", map[string]interface{}{"resource": customJson})
		s.Run("returns success", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(resourcesCreateOrUpdateCustom.IsError, "call tool failed, got: %v", resourcesCreateOrUpdateCustom.Content)
		})
		s.Run("creates custom resource", func() {
			dynamicClient := dynamic.NewForConfigOrDie(envTestRestConfig)
			_, err = dynamicClient.
				Resource(schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "customs"}).
				Namespace("default").
				Get(s.T().Context(), "a-custom-resource", metav1.GetOptions{})
			s.Nilf(err, "custom resource not found")
		})
	})

	s.Run("resources_create_or_update with valid namespaced json resource", func() {
		customJsonUpdated := "{\"apiVersion\": \"example.com/v1\", \"kind\": \"Custom\", \"metadata\": {\"name\": \"a-custom-resource\",\"annotations\": {\"updated\": \"true\"}}}"
		resourcesCreateOrUpdateCustomUpdated, err := s.CallTool("resources_create_or_update", map[string]interface{}{"resource": customJsonUpdated})
		s.Run("returns success", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(resourcesCreateOrUpdateCustomUpdated.IsError, "call tool failed")
		})
		s.Run("updates custom resource", func() {
			dynamicClient := dynamic.NewForConfigOrDie(envTestRestConfig)
			customResource, _ := dynamicClient.
				Resource(schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "customs"}).
				Namespace("default").
				Get(s.T().Context(), "a-custom-resource", metav1.GetOptions{})
			s.NotNil(customResource, "custom resource not found")
			annotations := customResource.GetAnnotations()
			s.Require().NotNil(annotations, "annotations should not be nil")
			s.Equalf("true", annotations["updated"], "custom resource not updated")
		})
	})
}

func (s *ResourcesSuite) TestResourcesCreateOrUpdateDenied() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		denied_resources = [
			{ version = "v1", kind = "Secret" },
			{ group = "rbac.authorization.k8s.io", version = "v1" }
		]
	`), s.Cfg), "Expected to parse denied resources config")
	s.InitMcpClient()
	s.Run("resources_create_or_update (denied by kind)", func() {
		secretYaml := "apiVersion: v1\nkind: Secret\nmetadata:\n  name: a-denied-secret\n  namespace: default\n"
		deniedByKind, err := s.CallTool("resources_create_or_update", map[string]interface{}{"resource": secretYaml})
		s.Run("has error", func() {
			s.Truef(deniedByKind.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			msg := deniedByKind.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			expectedMessage := "failed to create or update resources:(.+:)? resource not allowed: /v1, Kind=Secret"
			s.Regexpf(expectedMessage, msg,
				"expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
	s.Run("resources_create_or_update (denied by group)", func() {
		roleYaml := "apiVersion: rbac.authorization.k8s.io/v1\nkind: Role\nmetadata:\n  name: a-denied-role\n  namespace: default\n"
		deniedByGroup, err := s.CallTool("resources_create_or_update", map[string]interface{}{"resource": roleYaml})
		s.Run("has error", func() {
			s.Truef(deniedByGroup.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			msg := deniedByGroup.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			expectedMessage := "failed to create or update resources:(.+:)? resource not allowed: rbac.authorization.k8s.io/v1, Kind=Role"
			s.Regexpf(expectedMessage, msg,
				"expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
	s.Run("resources_create_or_update (not denied) creates or updates resource", func() {
		configMapYaml := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a-cm-created-or-updated\n  namespace: default\n"
		allowedResource, err := s.CallTool("resources_create_or_update", map[string]interface{}{"resource": configMapYaml})
		s.Falsef(allowedResource.IsError, "call tool should not fail")
		s.Nilf(err, "call tool should not return error object")
	})
}

func (s *ResourcesSuite) TestResourcesCreateOrUpdateForbidden() {
	s.InitMcpClient()
	defer restoreAuth(s.T().Context())
	client := kubernetes.NewForConfigOrDie(envTestRestConfig)
	// Remove all permissions - user will have forbidden access
	_ = client.RbacV1().ClusterRoles().Delete(s.T().Context(), "allow-all", metav1.DeleteOptions{})

	s.Run("resources_create_or_update (forbidden)", func() {
		capture := s.StartCapturingLogNotifications()
		configMapYaml := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a-forbidden-configmap\n  namespace: default\n"
		toolResult, _ := s.CallTool("resources_create_or_update", map[string]interface{}{"resource": configMapYaml})
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

func (s *ResourcesSuite) TestResourcesDelete() {
	s.InitMcpClient()
	client := kubernetes.NewForConfigOrDie(envTestRestConfig)

	s.Run("resources_delete with missing apiVersion returns error", func() {
		toolResult, _ := s.CallTool("resources_delete", map[string]interface{}{})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf("failed to delete resource, missing argument apiVersion", toolResult.Content[0].(mcp.TextContent).Text,
			"invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})
	s.Run("resources_delete with missing kind returns error", func() {
		toolResult, _ := s.CallTool("resources_delete", map[string]interface{}{"apiVersion": "v1"})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf("failed to delete resource, missing argument kind", toolResult.Content[0].(mcp.TextContent).Text,
			"invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})
	s.Run("resources_delete with invalid apiVersion returns error", func() {
		toolResult, _ := s.CallTool("resources_delete", map[string]interface{}{"apiVersion": "invalid/api/version", "kind": "Pod", "name": "a-pod"})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf("failed to delete resource, invalid argument apiVersion", toolResult.Content[0].(mcp.TextContent).Text,
			"invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})
	s.Run("resources_delete with nonexistent apiVersion returns error", func() {
		toolResult, _ := s.CallTool("resources_delete", map[string]interface{}{"apiVersion": "custom.non.existent.example.com/v1", "kind": "Custom", "name": "a-custom"})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf(`failed to delete resource: no matches for kind "Custom" in version "custom.non.existent.example.com/v1"`,
			toolResult.Content[0].(mcp.TextContent).Text, "invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})
	s.Run("resources_delete with missing name returns error", func() {
		toolResult, _ := s.CallTool("resources_delete", map[string]interface{}{"apiVersion": "v1", "kind": "Namespace"})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf("failed to delete resource, missing argument name", toolResult.Content[0].(mcp.TextContent).Text,
			"invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})
	s.Run("resources_delete with nonexistent resource", func() {
		capture := s.StartCapturingLogNotifications()
		toolResult, _ := s.CallTool("resources_delete", map[string]interface{}{"apiVersion": "v1", "kind": "ConfigMap", "name": "nonexistent-configmap"})
		s.Run("returns error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
			s.Equalf(`failed to delete resource: configmaps "nonexistent-configmap" not found`,
				toolResult.Content[0].(mcp.TextContent).Text, "invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
		})
		s.Run("sends log notification", func() {
			logNotification := capture.RequireLogNotification(s.T(), 2*time.Second)
			s.Equal("info", logNotification.Level, "not found errors should log at info level")
			s.Contains(logNotification.Data, "Resource not found", "log message should indicate resource not found")
		})
	})

	s.Run("resources_delete with valid namespaced resource", func() {
		resourcesDeleteCm, err := s.CallTool("resources_delete", map[string]interface{}{"apiVersion": "v1", "kind": "ConfigMap", "name": "a-configmap-to-delete"})
		s.Run("returns success", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(resourcesDeleteCm.IsError, "call tool failed")
			s.Equalf("Resource deleted successfully", resourcesDeleteCm.Content[0].(mcp.TextContent).Text,
				"invalid tool result content got: %v", resourcesDeleteCm.Content[0].(mcp.TextContent).Text)
		})
		s.Run("deletes ConfigMap", func() {
			_, err := client.CoreV1().ConfigMaps("default").Get(s.T().Context(), "a-configmap-to-delete", metav1.GetOptions{})
			s.Error(err, "ConfigMap not deleted")
		})
	})

	s.Run("resources_delete with valid cluster scoped resource", func() {
		resourcesDeleteNamespace, err := s.CallTool("resources_delete", map[string]interface{}{"apiVersion": "v1", "kind": "Namespace", "name": "ns-to-delete"})
		s.Run("returns success", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(resourcesDeleteNamespace.IsError, "call tool failed")
			s.Equalf("Resource deleted successfully", resourcesDeleteNamespace.Content[0].(mcp.TextContent).Text,
				"invalid tool result content got: %v", resourcesDeleteNamespace.Content[0].(mcp.TextContent).Text)
		})
		s.Run(" deletes Namespace", func() {
			ns, err := client.CoreV1().Namespaces().Get(s.T().Context(), "ns-to-delete", metav1.GetOptions{})
			s.Truef(err != nil || (ns != nil && ns.DeletionTimestamp != nil), "Namespace not deleted")
		})
	})
}

func (s *ResourcesSuite) TestResourcesDeleteDenied() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		denied_resources = [
			{ version = "v1", kind = "Secret" },
			{ group = "rbac.authorization.k8s.io", version = "v1" }
		]
	`), s.Cfg), "Expected to parse denied resources config")
	s.InitMcpClient()
	kc := kubernetes.NewForConfigOrDie(envTestRestConfig)
	_, _ = kc.CoreV1().ConfigMaps("default").Create(s.T().Context(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "allowed-configmap-to-delete"},
	}, metav1.CreateOptions{})
	s.Run("resources_delete (denied by kind)", func() {
		deniedByKind, err := s.CallTool("resources_delete", map[string]interface{}{"apiVersion": "v1", "kind": "Secret", "namespace": "default", "name": "denied-secret"})
		s.Run("has error", func() {
			s.Truef(deniedByKind.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			msg := deniedByKind.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			expectedMessage := "failed to delete resource:(.+:)? resource not allowed: /v1, Kind=Secret"
			s.Regexpf(expectedMessage, msg,
				"expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
	s.Run("resources_delete (denied by group)", func() {
		deniedByGroup, err := s.CallTool("resources_delete", map[string]interface{}{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role", "namespace": "default", "name": "denied-role"})
		s.Run("has error", func() {
			s.Truef(deniedByGroup.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			msg := deniedByGroup.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			expectedMessage := "failed to delete resource:(.+:)? resource not allowed: rbac.authorization.k8s.io/v1, Kind=Role"
			s.Regexpf(expectedMessage, msg,
				"expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
	s.Run("resources_delete (not denied) deletes resource", func() {
		allowedResource, err := s.CallTool("resources_delete", map[string]interface{}{"apiVersion": "v1", "kind": "ConfigMap", "name": "allowed-configmap-to-delete"})
		s.Falsef(allowedResource.IsError, "call tool should not fail")
		s.Nilf(err, "call tool should not return error object")
	})
}

func (s *ResourcesSuite) TestResourcesScale() {
	s.InitMcpClient()
	kc := kubernetes.NewForConfigOrDie(envTestRestConfig)
	deploymentName := "deployment-to-scale"
	_, _ = kc.AppsV1().Deployments("default").Create(s.T().Context(), &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: deploymentName},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(2)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": deploymentName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": deploymentName}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}},
				},
			},
		},
	}, metav1.CreateOptions{})

	s.Run("resources_scale with missing apiVersion returns error", func() {
		toolResult, _ := s.CallTool("resources_scale", map[string]interface{}{})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf("failed to get/update resource scale, missing argument apiVersion", toolResult.Content[0].(mcp.TextContent).Text,
			"invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})
	s.Run("resources_scale with missing kind returns error", func() {
		toolResult, _ := s.CallTool("resources_scale", map[string]interface{}{"apiVersion": "apps/v1"})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf("failed to get/update resource scale, missing argument kind", toolResult.Content[0].(mcp.TextContent).Text,
			"invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})
	s.Run("resources_scale with missing name returns error", func() {
		toolResult, _ := s.CallTool("resources_scale", map[string]interface{}{"apiVersion": "apps/v1", "kind": "Deployment"})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf("failed to get/update resource scale, missing argument name", toolResult.Content[0].(mcp.TextContent).Text,
			"invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})
	s.Run("resources_scale get returns current scale", func() {
		result, err := s.CallTool("resources_scale", map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"namespace":  "default",
			"name":       deploymentName,
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(result.IsError, "call tool failed: %v", result.Content)
		})
		s.Run("returns scale yaml", func() {
			content := result.Content[0].(mcp.TextContent).Text
			s.Truef(strings.HasPrefix(content, "# Current resource scale (YAML) is below"),
				"Expected success message, got %v", content)
			var decodedScale unstructured.Unstructured
			err = yaml.Unmarshal([]byte(strings.TrimPrefix(content, "# Current resource scale (YAML) is below\n")), &decodedScale)
			s.Nilf(err, "invalid tool result content %v", err)
			replicas, found, _ := unstructured.NestedInt64(decodedScale.Object, "spec", "replicas")
			s.Truef(found, "replicas not found in scale object")
			s.Equalf(int64(2), replicas, "expected 2 replicas, got %d", replicas)
		})
	})
	s.Run("resources_scale update changes the scale", func() {
		result, err := s.CallTool("resources_scale", map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"namespace":  "default",
			"name":       deploymentName,
			"scale":      5,
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(result.IsError, "call tool failed: %v", result.Content)
		})
		s.Run("returns updated scale yaml", func() {
			content := result.Content[0].(mcp.TextContent).Text
			var decodedScale unstructured.Unstructured
			err = yaml.Unmarshal([]byte(strings.TrimPrefix(content, "# Current resource scale (YAML) is below\n")), &decodedScale)
			s.Nilf(err, "invalid tool result content %v", err)
			replicas, found, _ := unstructured.NestedInt64(decodedScale.Object, "spec", "replicas")
			s.Truef(found, "replicas not found in scale object")
			s.Equalf(int64(5), replicas, "expected 5 replicas after update, got %d", replicas)
		})
		s.Run("deployment was actually scaled", func() {
			deployment, _ := kc.AppsV1().Deployments("default").Get(s.T().Context(), deploymentName, metav1.GetOptions{})
			s.Equalf(int32(5), *deployment.Spec.Replicas, "expected 5 replicas in deployment, got %d", *deployment.Spec.Replicas)
		})
	})
	s.Run("resources_scale with nonexistent resource", func() {
		capture := s.StartCapturingLogNotifications()
		toolResult, _ := s.CallTool("resources_scale", map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"namespace":  "default",
			"name":       "nonexistent-deployment",
		})
		s.Run("returns error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
			s.Containsf(toolResult.Content[0].(mcp.TextContent).Text, "not found",
				"expected not found error, got %v", toolResult.Content[0].(mcp.TextContent).Text)
		})
		s.Run("sends log notification", func() {
			logNotification := capture.RequireLogNotification(s.T(), 2*time.Second)
			s.Equal("info", logNotification.Level, "not found errors should log at info level")
			s.Contains(logNotification.Data, "Resource not found", "log message should indicate resource not found")
		})
	})
	s.Run("resources_scale with resource that does not support scale subresource returns error", func() {
		configMapName := "configmap-without-scale"
		_, _ = kc.CoreV1().ConfigMaps("default").Create(s.T().Context(), &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: configMapName},
			Data:       map[string]string{"key": "value"},
		}, metav1.CreateOptions{})
		toolResult, _ := s.CallTool("resources_scale", map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"namespace":  "default",
			"name":       configMapName,
		})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Containsf(toolResult.Content[0].(mcp.TextContent).Text, "the server could not find the requested resource",
			"expected scale subresource not found error, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})
}

func (s *ResourcesSuite) TestResourcesScaleDenied() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		denied_resources = [
			{ group = "apps", version = "v1" },
			{ group = "", version = "v1", kind = "ReplicationController" }
		]
	`), s.Cfg), "Expected to parse denied resources config")
	s.InitMcpClient()
	s.Run("resources_scale get (denied by kind)", func() {
		deniedByKind, err := s.CallTool("resources_scale", map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ReplicationController",
			"namespace":  "default",
			"name":       "nonexistent-rc",
		})
		s.Run("has error", func() {
			s.Truef(deniedByKind.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			msg := deniedByKind.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			expectedMessage := "failed to get/update resource scale:(.+:)? resource not allowed: /v1, Kind=ReplicationController"
			s.Regexpf(expectedMessage, msg,
				"expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
	s.Run("resources_scale update (denied by kind)", func() {
		deniedByKind, err := s.CallTool("resources_scale", map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ReplicationController",
			"namespace":  "default",
			"name":       "nonexistent-rc",
			"scale":      1337,
		})
		s.Run("has error", func() {
			s.Truef(deniedByKind.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			msg := deniedByKind.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			expectedMessage := "failed to get/update resource scale:(.+:)? resource not allowed: /v1, Kind=ReplicationController"
			s.Regexpf(expectedMessage, msg,
				"expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
	s.Run("resources_scale get (denied by group)", func() {
		deniedByGroup, err := s.CallTool("resources_scale", map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "StatefulSet",
			"namespace":  "default",
			"name":       "nonexistent-statefulset",
		})
		s.Run("has error", func() {
			s.Truef(deniedByGroup.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			msg := deniedByGroup.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			expectedMessage := "failed to get/update resource scale:(.+:)? resource not allowed: apps/v1, Kind=StatefulSet"
			s.Regexpf(expectedMessage, msg,
				"expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
	s.Run("resources_scale update (denied by group)", func() {
		deniedByGroup, err := s.CallTool("resources_scale", map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "StatefulSet",
			"namespace":  "default",
			"name":       "nonexistent-statefulset",
			"scale":      1337,
		})
		s.Run("has error", func() {
			s.Truef(deniedByGroup.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			msg := deniedByGroup.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			expectedMessage := "failed to get/update resource scale:(.+:)? resource not allowed: apps/v1, Kind=StatefulSet"
			s.Regexpf(expectedMessage, msg,
				"expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
}

func TestResources(t *testing.T) {
	suite.Run(t, new(ResourcesSuite))
}
