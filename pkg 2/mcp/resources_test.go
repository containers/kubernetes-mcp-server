package mcp

import (
	"regexp"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
)

func TestResourcesList(t *testing.T) {
	testCase(t, func(c *mcpContext) {
		c.withEnvTest()
		t.Run("resources_list with missing apiVersion returns error", func(t *testing.T) {
			toolResult, _ := c.callTool("resources_list", map[string]interface{}{})
			if !toolResult.IsError {
				t.Fatalf("call tool should fail")
			}
			if toolResult.Content[0].(mcp.TextContent).Text != "failed to list resources, missing argument apiVersion" {
				t.Fatalf("invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
			}
		})
		t.Run("resources_list with missing kind returns error", func(t *testing.T) {
			toolResult, _ := c.callTool("resources_list", map[string]interface{}{"apiVersion": "v1"})
			if !toolResult.IsError {
				t.Fatalf("call tool should fail")
			}
			if toolResult.Content[0].(mcp.TextContent).Text != "failed to list resources, missing argument kind" {
				t.Fatalf("invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
			}
		})
		t.Run("resources_list with invalid apiVersion returns error", func(t *testing.T) {
			toolResult, _ := c.callTool("resources_list", map[string]interface{}{"apiVersion": "invalid/api/version", "kind": "Pod"})
			if !toolResult.IsError {
				t.Fatalf("call tool should fail")
			}
			if toolResult.Content[0].(mcp.TextContent).Text != "failed to list resources, invalid argument apiVersion" {
				t.Fatalf("invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
			}
		})
		t.Run("resources_list with nonexistent apiVersion returns error", func(t *testing.T) {
			toolResult, _ := c.callTool("resources_list", map[string]interface{}{"apiVersion": "custom.non.existent.example.com/v1", "kind": "Custom"})
			if !toolResult.IsError {
				t.Fatalf("call tool should fail")
			}
			if toolResult.Content[0].(mcp.TextContent).Text != `failed to list resources: no matches for kind "Custom" in version "custom.non.existent.example.com/v1"` {
				t.Fatalf("invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
			}
		})
		namespaces, err := c.callTool("resources_list", map[string]interface{}{"apiVersion": "v1", "kind": "Namespace"})
		t.Run("resources_list returns namespaces", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
				return
			}
			if namespaces.IsError {
				t.Fatalf("call tool failed")
				return
			}
		})
		var decodedNamespaces []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(namespaces.Content[0].(mcp.TextContent).Text), &decodedNamespaces)
		t.Run("resources_list has yaml content", func(t *testing.T) {
			if err != nil {
				t.Fatalf("invalid tool result content %v", err)
			}
		})
		t.Run("resources_list returns more than 2 items", func(t *testing.T) {
			if len(decodedNamespaces) < 3 {
				t.Fatalf("invalid namespace count, expected >2, got %v", len(decodedNamespaces))
			}
		})

		// Test label selector functionality
		t.Run("resources_list with label selector returns filtered pods", func(t *testing.T) {

			// List pods with label selector
			result, err := c.callTool("resources_list", map[string]interface{}{
				"apiVersion":    "v1",
				"kind":          "Pod",
				"namespace":     "default",
				"labelSelector": "app=nginx",
			})

			if err != nil {
				t.Fatalf("call tool failed %v", err)
				return
			}
			if result.IsError {
				t.Fatalf("call tool failed")
				return
			}

			var decodedPods []unstructured.Unstructured
			err = yaml.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &decodedPods)
			if err != nil {
				t.Fatalf("invalid tool result content %v", err)
				return
			}

			// Verify only the pod with matching label is returned
			if len(decodedPods) != 1 {
				t.Fatalf("expected 1 pod, got %d", len(decodedPods))
				return
			}

			if decodedPods[0].GetName() != "a-pod-in-default" {
				t.Fatalf("expected pod-with-label, got %s", decodedPods[0].GetName())
				return
			}

			// Test that multiple label selectors work
			result, err = c.callTool("resources_list", map[string]interface{}{
				"apiVersion":    "v1",
				"kind":          "Pod",
				"namespace":     "default",
				"labelSelector": "test-label=test-value,another=value",
			})

			if err != nil {
				t.Fatalf("call tool failed %v", err)
				return
			}
			if result.IsError {
				t.Fatalf("call tool failed")
				return
			}

			err = yaml.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &decodedPods)
			if err != nil {
				t.Fatalf("invalid tool result content %v", err)
				return
			}

			// Verify no pods match multiple label selector
			if len(decodedPods) != 0 {
				t.Fatalf("expected 0 pods, got %d", len(decodedPods))
				return
			}
		})
	})
}

func TestResourcesListDenied(t *testing.T) {
	deniedResourcesServer := &config.StaticConfig{
		DeniedResources: []config.GroupVersionKind{
			{Version: "v1", Kind: "Secret"},
			{Group: "rbac.authorization.k8s.io", Version: "v1"},
		},
	}
	testCaseWithContext(t, &mcpContext{staticConfig: deniedResourcesServer}, func(c *mcpContext) {
		c.withEnvTest()
		deniedByKind, _ := c.callTool("resources_list", map[string]interface{}{"apiVersion": "v1", "kind": "Secret"})
		t.Run("resources_list (denied by kind) has error", func(t *testing.T) {
			if !deniedByKind.IsError {
				t.Fatalf("call tool should fail")
			}
		})
		t.Run("resources_list (denied by kind) describes denial", func(t *testing.T) {
			expectedMessage := "failed to list resources: resource not allowed: /v1, Kind=Secret"
			if deniedByKind.Content[0].(mcp.TextContent).Text != expectedMessage {
				t.Fatalf("expected descriptive error '%s', got %v", expectedMessage, deniedByKind.Content[0].(mcp.TextContent).Text)
			}
		})
		deniedByGroup, _ := c.callTool("resources_list", map[string]interface{}{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role"})
		t.Run("resources_list (denied by group) has error", func(t *testing.T) {
			if !deniedByGroup.IsError {
				t.Fatalf("call tool should fail")
			}
		})
		t.Run("resources_list (denied by group) describes denial", func(t *testing.T) {
			expectedMessage := "failed to list resources: resource not allowed: rbac.authorization.k8s.io/v1, Kind=Role"
			if deniedByGroup.Content[0].(mcp.TextContent).Text != expectedMessage {
				t.Fatalf("expected descriptive error '%s', got %v", expectedMessage, deniedByKind.Content[0].(mcp.TextContent).Text)
			}
		})
		allowedResource, _ := c.callTool("resources_list", map[string]interface{}{"apiVersion": "v1", "kind": "Namespace"})
		t.Run("resources_list (not denied) returns list", func(t *testing.T) {
			if allowedResource.IsError {
				t.Fatalf("call tool should not fail")
			}
		})
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

func TestResourcesGet(t *testing.T) {
	testCase(t, func(c *mcpContext) {
		c.withEnvTest()
		t.Run("resources_get with missing apiVersion returns error", func(t *testing.T) {
			toolResult, _ := c.callTool("resources_get", map[string]interface{}{})
			if !toolResult.IsError {
				t.Fatalf("call tool should fail")
				return
			}
			if toolResult.Content[0].(mcp.TextContent).Text != "failed to get resource, missing argument apiVersion" {
				t.Fatalf("invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
				return
			}
		})
		t.Run("resources_get with missing kind returns error", func(t *testing.T) {
			toolResult, _ := c.callTool("resources_get", map[string]interface{}{"apiVersion": "v1"})
			if !toolResult.IsError {
				t.Fatalf("call tool should fail")
				return
			}
			if toolResult.Content[0].(mcp.TextContent).Text != "failed to get resource, missing argument kind" {
				t.Fatalf("invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
				return
			}
		})
		t.Run("resources_get with invalid apiVersion returns error", func(t *testing.T) {
			toolResult, _ := c.callTool("resources_get", map[string]interface{}{"apiVersion": "invalid/api/version", "kind": "Pod", "name": "a-pod"})
			if !toolResult.IsError {
				t.Fatalf("call tool should fail")
				return
			}
			if toolResult.Content[0].(mcp.TextContent).Text != "failed to get resource, invalid argument apiVersion" {
				t.Fatalf("invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
				return
			}
		})
		t.Run("resources_get with nonexistent apiVersion returns error", func(t *testing.T) {
			toolResult, _ := c.callTool("resources_get", map[string]interface{}{"apiVersion": "custom.non.existent.example.com/v1", "kind": "Custom", "name": "a-custom"})
			if !toolResult.IsError {
				t.Fatalf("call tool should fail")
				return
			}
			if toolResult.Content[0].(mcp.TextContent).Text != `failed to get resource: no matches for kind "Custom" in version "custom.non.existent.example.com/v1"` {
				t.Fatalf("invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
				return
			}
		})
		t.Run("resources_get with missing name returns error", func(t *testing.T) {
			toolResult, _ := c.callTool("resources_get", map[string]interface{}{"apiVersion": "v1", "kind": "Namespace"})
			if !toolResult.IsError {
				t.Fatalf("call tool should fail")
				return
			}
			if toolResult.Content[0].(mcp.TextContent).Text != "failed to get resource, missing argument name" {
				t.Fatalf("invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
				return
			}
		})
		namespace, err := c.callTool("resources_get", map[string]interface{}{"apiVersion": "v1", "kind": "Namespace", "name": "default"})
		t.Run("resources_get returns namespace", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
				return
			}
			if namespace.IsError {
				t.Fatalf("call tool failed")
				return
			}
		})
		var decodedNamespace unstructured.Unstructured
		err = yaml.Unmarshal([]byte(namespace.Content[0].(mcp.TextContent).Text), &decodedNamespace)
		t.Run("resources_get has yaml content", func(t *testing.T) {
			if err != nil {
				t.Fatalf("invalid tool result content %v", err)
				return
			}
		})
		t.Run("resources_get returns default namespace", func(t *testing.T) {
			if decodedNamespace.GetName() != "default" {
				t.Fatalf("invalid namespace name, expected default, got %v", decodedNamespace.GetName())
				return
			}
		})
	})
}

func TestResourcesGetDenied(t *testing.T) {
	deniedResourcesServer := &config.StaticConfig{
		DeniedResources: []config.GroupVersionKind{
			{Version: "v1", Kind: "Secret"},
			{Group: "rbac.authorization.k8s.io", Version: "v1"},
		},
	}
	testCaseWithContext(t, &mcpContext{staticConfig: deniedResourcesServer}, func(c *mcpContext) {
		c.withEnvTest()
		kc := c.newKubernetesClient()
		_, _ = kc.CoreV1().Secrets("default").Create(c.ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "denied-secret"},
		}, metav1.CreateOptions{})
		_, _ = kc.RbacV1().Roles("default").Create(c.ctx, &v1.Role{
			ObjectMeta: metav1.ObjectMeta{Name: "denied-role"},
		}, metav1.CreateOptions{})
		deniedByKind, _ := c.callTool("resources_get", map[string]interface{}{"apiVersion": "v1", "kind": "Secret", "namespace": "default", "name": "denied-secret"})
		t.Run("resources_get (denied by kind) has error", func(t *testing.T) {
			if !deniedByKind.IsError {
				t.Fatalf("call tool should fail")
			}
		})
		t.Run("resources_get (denied by kind) describes denial", func(t *testing.T) {
			expectedMessage := "failed to get resource: resource not allowed: /v1, Kind=Secret"
			if deniedByKind.Content[0].(mcp.TextContent).Text != expectedMessage {
				t.Fatalf("expected descriptive error '%s', got %v", expectedMessage, deniedByKind.Content[0].(mcp.TextContent).Text)
			}
		})
		deniedByGroup, _ := c.callTool("resources_get", map[string]interface{}{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role", "namespace": "default", "name": "denied-role"})
		t.Run("resources_get (denied by group) has error", func(t *testing.T) {
			if !deniedByGroup.IsError {
				t.Fatalf("call tool should fail")
			}
		})
		t.Run("resources_get (denied by group) describes denial", func(t *testing.T) {
			expectedMessage := "failed to get resource: resource not allowed: rbac.authorization.k8s.io/v1, Kind=Role"
			if deniedByGroup.Content[0].(mcp.TextContent).Text != expectedMessage {
				t.Fatalf("expected descriptive error '%s', got %v", expectedMessage, deniedByKind.Content[0].(mcp.TextContent).Text)
			}
		})
		allowedResource, _ := c.callTool("resources_get", map[string]interface{}{"apiVersion": "v1", "kind": "Namespace", "name": "default"})
		t.Run("resources_get (not denied) returns resource", func(t *testing.T) {
			if allowedResource.IsError {
				t.Fatalf("call tool should not fail")
			}
		})
	})
}

func TestResourcesCreateOrUpdate(t *testing.T) {
	testCase(t, func(c *mcpContext) {
		c.withEnvTest()
		t.Run("resources_create_or_update with nil resource returns error", func(t *testing.T) {
			toolResult, _ := c.callTool("resources_create_or_update", map[string]interface{}{})
			if toolResult.IsError != true {
				t.Fatalf("call tool should fail")
				return
			}
			if toolResult.Content[0].(mcp.TextContent).Text != "failed to create or update resources, missing argument resource" {
				t.Fatalf("invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
				return
			}
		})
		t.Run("resources_create_or_update with empty resource returns error", func(t *testing.T) {
			toolResult, _ := c.callTool("resources_create_or_update", map[string]interface{}{"resource": ""})
			if toolResult.IsError != true {
				t.Fatalf("call tool should fail")
				return
			}
			if toolResult.Content[0].(mcp.TextContent).Text != "failed to create or update resources, missing argument resource" {
				t.Fatalf("invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
				return
			}
		})
		client := c.newKubernetesClient()
		configMapYaml := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a-cm-created-or-updated\n  namespace: default\n"
		resourcesCreateOrUpdateCm1, err := c.callTool("resources_create_or_update", map[string]interface{}{"resource": configMapYaml})
		t.Run("resources_create_or_update with valid namespaced yaml resource returns success", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
				return
			}
			if resourcesCreateOrUpdateCm1.IsError {
				t.Errorf("call tool failed")
				return
			}
		})
		var decodedCreateOrUpdateCm1 []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(resourcesCreateOrUpdateCm1.Content[0].(mcp.TextContent).Text), &decodedCreateOrUpdateCm1)
		t.Run("resources_create_or_update with valid namespaced yaml resource returns yaml content", func(t *testing.T) {
			if err != nil {
				t.Errorf("invalid tool result content %v", err)
				return
			}
			if !strings.HasPrefix(resourcesCreateOrUpdateCm1.Content[0].(mcp.TextContent).Text, "# The following resources (YAML) have been created or updated successfully") {
				t.Errorf("Excpected success message, got %v", resourcesCreateOrUpdateCm1.Content[0].(mcp.TextContent).Text)
				return
			}
			if len(decodedCreateOrUpdateCm1) != 1 {
				t.Errorf("invalid resource count, expected 1, got %v", len(decodedCreateOrUpdateCm1))
				return
			}
			if decodedCreateOrUpdateCm1[0].GetName() != "a-cm-created-or-updated" {
				t.Errorf("invalid resource name, expected a-cm-created-or-updated, got %v", decodedCreateOrUpdateCm1[0].GetName())
				return
			}
			if decodedCreateOrUpdateCm1[0].GetUID() == "" {
				t.Errorf("invalid uid, got %v", decodedCreateOrUpdateCm1[0].GetUID())
				return
			}
		})
		t.Run("resources_create_or_update with valid namespaced yaml resource creates ConfigMap", func(t *testing.T) {
			cm, _ := client.CoreV1().ConfigMaps("default").Get(c.ctx, "a-cm-created-or-updated", metav1.GetOptions{})
			if cm == nil {
				t.Fatalf("ConfigMap not found")
				return
			}
		})
		configMapJson := "{\"apiVersion\": \"v1\", \"kind\": \"ConfigMap\", \"metadata\": {\"name\": \"a-cm-created-or-updated-2\", \"namespace\": \"default\"}}"
		resourcesCreateOrUpdateCm2, err := c.callTool("resources_create_or_update", map[string]interface{}{"resource": configMapJson})
		t.Run("resources_create_or_update with valid namespaced json resource returns success", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
				return
			}
			if resourcesCreateOrUpdateCm2.IsError {
				t.Fatalf("call tool failed")
				return
			}
		})
		t.Run("resources_create_or_update with valid namespaced json resource creates config map", func(t *testing.T) {
			cm, _ := client.CoreV1().ConfigMaps("default").Get(c.ctx, "a-cm-created-or-updated-2", metav1.GetOptions{})
			if cm == nil {
				t.Fatalf("ConfigMap not found")
				return
			}
		})
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
		resourcesCreateOrUpdateCrd, err := c.callTool("resources_create_or_update", map[string]interface{}{"resource": customResourceDefinitionJson})
		t.Run("resources_create_or_update with valid cluster-scoped json resource returns success", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
				return
			}
			if resourcesCreateOrUpdateCrd.IsError {
				t.Fatalf("call tool failed")
				return
			}
		})
		t.Run("resources_create_or_update with valid cluster-scoped json resource creates custom resource definition", func(t *testing.T) {
			apiExtensionsV1Client := c.newApiExtensionsClient()
			_, err = apiExtensionsV1Client.CustomResourceDefinitions().Get(c.ctx, "customs.example.com", metav1.GetOptions{})
			if err != nil {
				t.Fatalf("custom resource definition not found")
				return
			}
		})
		c.crdWaitUntilReady("customs.example.com")
		customJson := "{\"apiVersion\": \"example.com/v1\", \"kind\": \"Custom\", \"metadata\": {\"name\": \"a-custom-resource\"}}"
		resourcesCreateOrUpdateCustom, err := c.callTool("resources_create_or_update", map[string]interface{}{"resource": customJson})
		t.Run("resources_create_or_update with valid namespaced json resource returns success", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
				return
			}
			if resourcesCreateOrUpdateCustom.IsError {
				t.Fatalf("call tool failed, got: %v", resourcesCreateOrUpdateCustom.Content)
				return
			}
		})
		t.Run("resources_create_or_update with valid namespaced json resource creates custom resource", func(t *testing.T) {
			dynamicClient := dynamic.NewForConfigOrDie(envTestRestConfig)
			_, err = dynamicClient.
				Resource(schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "customs"}).
				Namespace("default").
				Get(c.ctx, "a-custom-resource", metav1.GetOptions{})
			if err != nil {
				t.Fatalf("custom resource not found")
				return
			}
		})
		customJsonUpdated := "{\"apiVersion\": \"example.com/v1\", \"kind\": \"Custom\", \"metadata\": {\"name\": \"a-custom-resource\",\"annotations\": {\"updated\": \"true\"}}}"
		resourcesCreateOrUpdateCustomUpdated, err := c.callTool("resources_create_or_update", map[string]interface{}{"resource": customJsonUpdated})
		t.Run("resources_create_or_update with valid namespaced json resource updates custom resource", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
				return
			}
			if resourcesCreateOrUpdateCustomUpdated.IsError {
				t.Fatalf("call tool failed")
				return
			}
		})
		t.Run("resources_create_or_update with valid namespaced json resource updates custom resource", func(t *testing.T) {
			dynamicClient := dynamic.NewForConfigOrDie(envTestRestConfig)
			customResource, _ := dynamicClient.
				Resource(schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "customs"}).
				Namespace("default").
				Get(c.ctx, "a-custom-resource", metav1.GetOptions{})
			if customResource == nil {
				t.Fatalf("custom resource not found")
				return
			}
			annotations := customResource.GetAnnotations()
			if annotations == nil || annotations["updated"] != "true" {
				t.Fatalf("custom resource not updated")
				return
			}
		})
	})
}

func TestResourcesCreateOrUpdateDenied(t *testing.T) {
	deniedResourcesServer := &config.StaticConfig{
		DeniedResources: []config.GroupVersionKind{
			{Version: "v1", Kind: "Secret"},
			{Group: "rbac.authorization.k8s.io", Version: "v1"},
		},
	}
	testCaseWithContext(t, &mcpContext{staticConfig: deniedResourcesServer}, func(c *mcpContext) {
		c.withEnvTest()
		secretYaml := "apiVersion: v1\nkind: Secret\nmetadata:\n  name: a-denied-secret\n  namespace: default\n"
		deniedByKind, _ := c.callTool("resources_create_or_update", map[string]interface{}{"resource": secretYaml})
		t.Run("resources_create_or_update (denied by kind) has error", func(t *testing.T) {
			if !deniedByKind.IsError {
				t.Fatalf("call tool should fail")
			}
		})
		t.Run("resources_create_or_update (denied by kind) describes denial", func(t *testing.T) {
			expectedMessage := "failed to create or update resources: resource not allowed: /v1, Kind=Secret"
			if deniedByKind.Content[0].(mcp.TextContent).Text != expectedMessage {
				t.Fatalf("expected descriptive error '%s', got %v", expectedMessage, deniedByKind.Content[0].(mcp.TextContent).Text)
			}
		})
		roleYaml := "apiVersion: rbac.authorization.k8s.io/v1\nkind: Role\nmetadata:\n  name: a-denied-role\n  namespace: default\n"
		deniedByGroup, _ := c.callTool("resources_create_or_update", map[string]interface{}{"resource": roleYaml})
		t.Run("resources_create_or_update (denied by group) has error", func(t *testing.T) {
			if !deniedByGroup.IsError {
				t.Fatalf("call tool should fail")
			}
		})
		t.Run("resources_create_or_update (denied by group) describes denial", func(t *testing.T) {
			expectedMessage := "failed to create or update resources: resource not allowed: rbac.authorization.k8s.io/v1, Kind=Role"
			if deniedByGroup.Content[0].(mcp.TextContent).Text != expectedMessage {
				t.Fatalf("expected descriptive error '%s', got %v", expectedMessage, deniedByKind.Content[0].(mcp.TextContent).Text)
			}
		})
		configMapYaml := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a-cm-created-or-updated\n  namespace: default\n"
		allowedResource, _ := c.callTool("resources_create_or_update", map[string]interface{}{"resource": configMapYaml})
		t.Run("resources_create_or_update (not denied) creates or updates resource", func(t *testing.T) {
			if allowedResource.IsError {
				t.Fatalf("call tool should not fail")
			}
		})
	})
}

func TestResourcesDelete(t *testing.T) {
	testCase(t, func(c *mcpContext) {
		c.withEnvTest()
		t.Run("resources_delete with missing apiVersion returns error", func(t *testing.T) {
			toolResult, _ := c.callTool("resources_delete", map[string]interface{}{})
			if !toolResult.IsError {
				t.Fatalf("call tool should fail")
				return
			}
			if toolResult.Content[0].(mcp.TextContent).Text != "failed to delete resource, missing argument apiVersion" {
				t.Fatalf("invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
				return
			}
		})
		t.Run("resources_delete with missing kind returns error", func(t *testing.T) {
			toolResult, _ := c.callTool("resources_delete", map[string]interface{}{"apiVersion": "v1"})
			if !toolResult.IsError {
				t.Fatalf("call tool should fail")
				return
			}
			if toolResult.Content[0].(mcp.TextContent).Text != "failed to delete resource, missing argument kind" {
				t.Fatalf("invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
				return
			}
		})
		t.Run("resources_delete with invalid apiVersion returns error", func(t *testing.T) {
			toolResult, _ := c.callTool("resources_delete", map[string]interface{}{"apiVersion": "invalid/api/version", "kind": "Pod", "name": "a-pod"})
			if !toolResult.IsError {
				t.Fatalf("call tool should fail")
				return
			}
			if toolResult.Content[0].(mcp.TextContent).Text != "failed to delete resource, invalid argument apiVersion" {
				t.Fatalf("invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
				return
			}
		})
		t.Run("resources_delete with nonexistent apiVersion returns error", func(t *testing.T) {
			toolResult, _ := c.callTool("resources_delete", map[string]interface{}{"apiVersion": "custom.non.existent.example.com/v1", "kind": "Custom", "name": "a-custom"})
			if !toolResult.IsError {
				t.Fatalf("call tool should fail")
				return
			}
			if toolResult.Content[0].(mcp.TextContent).Text != `failed to delete resource: no matches for kind "Custom" in version "custom.non.existent.example.com/v1"` {
				t.Fatalf("invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
				return
			}
		})
		t.Run("resources_delete with missing name returns error", func(t *testing.T) {
			toolResult, _ := c.callTool("resources_delete", map[string]interface{}{"apiVersion": "v1", "kind": "Namespace"})
			if !toolResult.IsError {
				t.Fatalf("call tool should fail")
				return
			}
			if toolResult.Content[0].(mcp.TextContent).Text != "failed to delete resource, missing argument name" {
				t.Fatalf("invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
				return
			}
		})
		t.Run("resources_delete with nonexistent resource returns error", func(t *testing.T) {
			toolResult, _ := c.callTool("resources_delete", map[string]interface{}{"apiVersion": "v1", "kind": "ConfigMap", "name": "nonexistent-configmap"})
			if !toolResult.IsError {
				t.Fatalf("call tool should fail")
				return
			}
			if toolResult.Content[0].(mcp.TextContent).Text != `failed to delete resource: configmaps "nonexistent-configmap" not found` {
				t.Fatalf("invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
				return
			}
		})
		resourcesDeleteCm, err := c.callTool("resources_delete", map[string]interface{}{"apiVersion": "v1", "kind": "ConfigMap", "name": "a-configmap-to-delete"})
		t.Run("resources_delete with valid namespaced resource returns success", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
				return
			}
			if resourcesDeleteCm.IsError {
				t.Fatalf("call tool failed")
				return
			}
			if resourcesDeleteCm.Content[0].(mcp.TextContent).Text != "Resource deleted successfully" {
				t.Fatalf("invalid tool result content got: %v", resourcesDeleteCm.Content[0].(mcp.TextContent).Text)
				return
			}
		})
		client := c.newKubernetesClient()
		t.Run("resources_delete with valid namespaced resource deletes ConfigMap", func(t *testing.T) {
			_, err := client.CoreV1().ConfigMaps("default").Get(c.ctx, "a-configmap-to-delete", metav1.GetOptions{})
			if err == nil {
				t.Fatalf("ConfigMap not deleted")
				return
			}
		})
		resourcesDeleteNamespace, err := c.callTool("resources_delete", map[string]interface{}{"apiVersion": "v1", "kind": "Namespace", "name": "ns-to-delete"})
		t.Run("resources_delete with valid namespaced resource returns success", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
				return
			}
			if resourcesDeleteNamespace.IsError {
				t.Fatalf("call tool failed")
				return
			}
			if resourcesDeleteNamespace.Content[0].(mcp.TextContent).Text != "Resource deleted successfully" {
				t.Fatalf("invalid tool result content got: %v", resourcesDeleteNamespace.Content[0].(mcp.TextContent).Text)
				return
			}
		})
		t.Run("resources_delete with valid namespaced resource deletes Namespace", func(t *testing.T) {
			ns, err := client.CoreV1().Namespaces().Get(c.ctx, "ns-to-delete", metav1.GetOptions{})
			if err == nil && ns != nil && ns.DeletionTimestamp == nil {
				t.Fatalf("Namespace not deleted")
				return
			}
		})
	})
}

func TestResourcesDeleteDenied(t *testing.T) {
	deniedResourcesServer := &config.StaticConfig{
		DeniedResources: []config.GroupVersionKind{
			{Version: "v1", Kind: "Secret"},
			{Group: "rbac.authorization.k8s.io", Version: "v1"},
		},
	}
	testCaseWithContext(t, &mcpContext{staticConfig: deniedResourcesServer}, func(c *mcpContext) {
		c.withEnvTest()
		kc := c.newKubernetesClient()
		_, _ = kc.CoreV1().ConfigMaps("default").Create(c.ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "allowed-configmap-to-delete"},
		}, metav1.CreateOptions{})
		deniedByKind, _ := c.callTool("resources_delete", map[string]interface{}{"apiVersion": "v1", "kind": "Secret", "namespace": "default", "name": "denied-secret"})
		t.Run("resources_delete (denied by kind) has error", func(t *testing.T) {
			if !deniedByKind.IsError {
				t.Fatalf("call tool should fail")
			}
		})
		t.Run("resources_delete (denied by kind) describes denial", func(t *testing.T) {
			expectedMessage := "failed to delete resource: resource not allowed: /v1, Kind=Secret"
			if deniedByKind.Content[0].(mcp.TextContent).Text != expectedMessage {
				t.Fatalf("expected descriptive error '%s', got %v", expectedMessage, deniedByKind.Content[0].(mcp.TextContent).Text)
			}
		})
		deniedByGroup, _ := c.callTool("resources_delete", map[string]interface{}{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role", "namespace": "default", "name": "denied-role"})
		t.Run("resources_delete (denied by group) has error", func(t *testing.T) {
			if !deniedByGroup.IsError {
				t.Fatalf("call tool should fail")
			}
		})
		t.Run("resources_delete (denied by group) describes denial", func(t *testing.T) {
			expectedMessage := "failed to delete resource: resource not allowed: rbac.authorization.k8s.io/v1, Kind=Role"
			if deniedByGroup.Content[0].(mcp.TextContent).Text != expectedMessage {
				t.Fatalf("expected descriptive error '%s', got %v", expectedMessage, deniedByKind.Content[0].(mcp.TextContent).Text)
			}
		})
		allowedResource, _ := c.callTool("resources_delete", map[string]interface{}{"apiVersion": "v1", "kind": "ConfigMap", "name": "allowed-configmap-to-delete"})
		t.Run("resources_delete (not denied) deletes resource", func(t *testing.T) {
			if allowedResource.IsError {
				t.Fatalf("call tool should not fail")
			}
		})
	})
}
