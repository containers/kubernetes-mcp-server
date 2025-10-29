package mcp

import (
	"regexp"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"

	"github.com/containers/kubernetes-mcp-server/pkg/output"
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
			expectedMessage := "failed to list resources: resource not allowed: /v1, Kind=Secret"
			s.Equalf(expectedMessage, deniedByKind.Content[0].(mcp.TextContent).Text,
				"expected descriptive error '%s', got %v", expectedMessage, deniedByKind.Content[0].(mcp.TextContent).Text)
		})
	})
	s.Run("resources_list (denied by group)", func() {
		deniedByGroup, err := s.CallTool("resources_list", map[string]interface{}{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role"})
		s.Run("has error", func() {
			s.Truef(deniedByGroup.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			expectedMessage := "failed to list resources: resource not allowed: rbac.authorization.k8s.io/v1, Kind=Role"
			s.Equalf(expectedMessage, deniedByGroup.Content[0].(mcp.TextContent).Text,
				"expected descriptive error '%s', got %v", expectedMessage, deniedByGroup.Content[0].(mcp.TextContent).Text)
		})
	})
	s.Run("resources_list (not denied) returns list", func() {
		allowedResource, _ := s.CallTool("resources_list", map[string]interface{}{"apiVersion": "v1", "kind": "Namespace"})
		s.Falsef(allowedResource.IsError, "call tool should not fail")
	})
}

func TestResourcesListAsTable(t *testing.T) {
	testCaseWithContext(t, &mcpContext{listOutput: output.Table, before: inOpenShift, after: inOpenShiftClear}, func(c *mcpContext) {
		c.withEnvTest()
		kc := c.newKubernetesClient()
		_, _ = kc.CoreV1().ConfigMaps("default").Create(t.Context(), &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "a-configmap-to-list-as-table", Labels: map[string]string{"resource": "config-map"}},
			Data:       map[string]string{"key": "value"},
		}, metav1.CreateOptions{})
		configMapList, err := c.callTool("resources_list", map[string]interface{}{"apiVersion": "v1", "kind": "ConfigMap"})
		t.Run("resources_list returns ConfigMap list", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
			}
			if configMapList.IsError {
				t.Fatalf("call tool failed")
			}
		})
		outConfigMapList := configMapList.Content[0].(mcp.TextContent).Text
		t.Run("resources_list returns column headers for ConfigMap list", func(t *testing.T) {
			expectedHeaders := "NAMESPACE\\s+APIVERSION\\s+KIND\\s+NAME\\s+DATA\\s+AGE\\s+LABELS"
			if m, e := regexp.MatchString(expectedHeaders, outConfigMapList); !m || e != nil {
				t.Fatalf("Expected headers '%s' not found in output:\n%s", expectedHeaders, outConfigMapList)
			}
		})
		t.Run("resources_list returns formatted row for a-configmap-to-list-as-table", func(t *testing.T) {
			expectedRow := "(?<namespace>default)\\s+" +
				"(?<apiVersion>v1)\\s+" +
				"(?<kind>ConfigMap)\\s+" +
				"(?<name>a-configmap-to-list-as-table)\\s+" +
				"(?<data>1)\\s+" +
				"(?<age>(\\d+m)?(\\d+s)?)\\s+" +
				"(?<labels>resource=config-map)"
			if m, e := regexp.MatchString(expectedRow, outConfigMapList); !m || e != nil {
				t.Fatalf("Expected row '%s' not found in output:\n%s", expectedRow, outConfigMapList)
			}
		})
		// Custom Resource List
		_, _ = dynamic.NewForConfigOrDie(envTestRestConfig).
			Resource(schema.GroupVersionResource{Group: "route.openshift.io", Version: "v1", Resource: "routes"}).
			Namespace("default").
			Create(c.ctx, &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "route.openshift.io/v1",
				"kind":       "Route",
				"metadata": map[string]interface{}{
					"name": "an-openshift-route-to-list-as-table",
				},
			}}, metav1.CreateOptions{})
		routeList, err := c.callTool("resources_list", map[string]interface{}{"apiVersion": "route.openshift.io/v1", "kind": "Route"})
		t.Run("resources_list returns Route list", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
			}
			if routeList.IsError {
				t.Fatalf("call tool failed")
			}
		})
		outRouteList := routeList.Content[0].(mcp.TextContent).Text
		t.Run("resources_list returns column headers for Route list", func(t *testing.T) {
			expectedHeaders := "NAMESPACE\\s+APIVERSION\\s+KIND\\s+NAME\\s+AGE\\s+LABELS"
			if m, e := regexp.MatchString(expectedHeaders, outRouteList); !m || e != nil {
				t.Fatalf("Expected headers '%s' not found in output:\n%s", expectedHeaders, outRouteList)
			}
		})
		t.Run("resources_list returns formatted row for an-openshift-route-to-list-as-table", func(t *testing.T) {
			expectedRow := "(?<namespace>default)\\s+" +
				"(?<apiVersion>route.openshift.io/v1)\\s+" +
				"(?<kind>Route)\\s+" +
				"(?<name>an-openshift-route-to-list-as-table)\\s+" +
				"(?<age>(\\d+m)?(\\d+s)?)\\s+" +
				"(?<labels><none>)"
			if m, e := regexp.MatchString(expectedRow, outRouteList); !m || e != nil {
				t.Fatalf("Expected row '%s' not found in output:\n%s", expectedRow, outRouteList)
			}
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
			expectedMessage := "failed to get resource: resource not allowed: /v1, Kind=Secret"
			s.Equalf(expectedMessage, deniedByKind.Content[0].(mcp.TextContent).Text,
				"expected descriptive error '%s', got %v", expectedMessage, deniedByKind.Content[0].(mcp.TextContent).Text)
		})
	})
	s.Run("resources_get (denied by group)", func() {
		deniedByGroup, err := s.CallTool("resources_get", map[string]interface{}{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role", "namespace": "default", "name": "denied-role"})
		s.Run("has error", func() {
			s.Truef(deniedByGroup.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			expectedMessage := "failed to get resource: resource not allowed: rbac.authorization.k8s.io/v1, Kind=Role"
			s.Equalf(expectedMessage, deniedByGroup.Content[0].(mcp.TextContent).Text,
				"expected descriptive error '%s', got %v", expectedMessage, deniedByGroup.Content[0].(mcp.TextContent).Text)
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
		s.CrdWaitUntilReady("customs.example.com")
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
			expectedMessage := "failed to create or update resources: resource not allowed: /v1, Kind=Secret"
			s.Equalf(expectedMessage, deniedByKind.Content[0].(mcp.TextContent).Text,
				"expected descriptive error '%s', got %v", expectedMessage, deniedByKind.Content[0].(mcp.TextContent).Text)
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
			expectedMessage := "failed to create or update resources: resource not allowed: rbac.authorization.k8s.io/v1, Kind=Role"
			s.Equalf(expectedMessage, deniedByGroup.Content[0].(mcp.TextContent).Text,
				"expected descriptive error '%s', got %v", expectedMessage, deniedByGroup.Content[0].(mcp.TextContent).Text)
		})
	})
	s.Run("resources_create_or_update (not denied) creates or updates resource", func() {
		configMapYaml := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a-cm-created-or-updated\n  namespace: default\n"
		allowedResource, err := s.CallTool("resources_create_or_update", map[string]interface{}{"resource": configMapYaml})
		s.Falsef(allowedResource.IsError, "call tool should not fail")
		s.Nilf(err, "call tool should not return error object")
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
	s.Run("resources_delete with nonexistent resource returns error", func() {
		toolResult, _ := s.CallTool("resources_delete", map[string]interface{}{"apiVersion": "v1", "kind": "ConfigMap", "name": "nonexistent-configmap"})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf(`failed to delete resource: configmaps "nonexistent-configmap" not found`,
			toolResult.Content[0].(mcp.TextContent).Text, "invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
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
			expectedMessage := "failed to delete resource: resource not allowed: /v1, Kind=Secret"
			s.Equalf(expectedMessage, deniedByKind.Content[0].(mcp.TextContent).Text,
				"expected descriptive error '%s', got %v", expectedMessage, deniedByKind.Content[0].(mcp.TextContent).Text)
		})
	})
	s.Run("resources_delete (denied by group)", func() {
		deniedByGroup, err := s.CallTool("resources_delete", map[string]interface{}{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role", "namespace": "default", "name": "denied-role"})
		s.Run("has error", func() {
			s.Truef(deniedByGroup.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			expectedMessage := "failed to delete resource: resource not allowed: rbac.authorization.k8s.io/v1, Kind=Role"
			s.Equalf(expectedMessage, deniedByGroup.Content[0].(mcp.TextContent).Text,
				"expected descriptive error '%s', got %v", expectedMessage, deniedByGroup.Content[0].(mcp.TextContent).Text)
		})
	})
	s.Run("resources_delete (not denied) deletes resource", func() {
		allowedResource, err := s.CallTool("resources_delete", map[string]interface{}{"apiVersion": "v1", "kind": "ConfigMap", "name": "allowed-configmap-to-delete"})
		s.Falsef(allowedResource.IsError, "call tool should not fail")
		s.Nilf(err, "call tool should not return error object")
	})
}

func TestResources(t *testing.T) {
	suite.Run(t, new(ResourcesSuite))
}
