package mcp

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/suite"

	"github.com/mark3labs/mcp-go/mcp"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"
)

type PodsSuite struct {
	BaseMcpSuite
}

func (s *PodsSuite) TestPodsListInAllNamespaces() {
	s.InitMcpClient()
	s.Run("pods_list returns pods list in all namespaces", func() {
		toolResult, err := s.CallTool("pods_list", map[string]interface{}{})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		var decoded []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decoded)
		s.Run("has yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
		})
		s.Run("returns at least 3 items", func() {
			s.GreaterOrEqualf(len(decoded), 3, "invalid pods count, expected at least 3, got %v", len(decoded))
		})
		var aPodInNs1, aPodInNs2 *unstructured.Unstructured
		for _, pod := range decoded {
			switch pod.GetName() {
			case "a-pod-in-ns-1":
				aPodInNs1 = &pod
			case "a-pod-in-ns-2":
				aPodInNs2 = &pod
			}
		}
		s.Run("returns pod in ns-1", func() {
			s.Require().NotNil(aPodInNs1, "aPodInNs1 is nil")
			s.Equalf("a-pod-in-ns-1", aPodInNs1.GetName(), "invalid pod name, expected a-pod-in-ns-1, got %v", aPodInNs1.GetName())
			s.Equalf("ns-1", aPodInNs1.GetNamespace(), "invalid pod namespace, expected ns-1, got %v", aPodInNs1.GetNamespace())
		})
		s.Run("returns pod in ns-2", func() {
			s.Require().NotNil(aPodInNs2, "aPodInNs2 is nil")
			s.Equalf("a-pod-in-ns-2", aPodInNs2.GetName(), "invalid pod name, expected a-pod-in-ns-2, got %v", aPodInNs2.GetName())
			s.Equalf("ns-2", aPodInNs2.GetNamespace(), "invalid pod namespace, expected ns-2, got %v", aPodInNs2.GetNamespace())
		})
		s.Run("omits managed fields", func() {
			s.Nilf(decoded[1].GetManagedFields(), "managed fields should be omitted, got %v", decoded[1].GetManagedFields())
		})
	})
}

func (s *PodsSuite) TestPodsListInAllNamespacesUnauthorized() {
	s.InitMcpClient()
	defer restoreAuth(s.T().Context())
	client := kubernetes.NewForConfigOrDie(envTestRestConfig)
	// Authorize user only for default/configured namespace
	r, _ := client.RbacV1().Roles("default").Create(s.T().Context(), &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-pods-list"},
		Rules: []rbacv1.PolicyRule{{
			Verbs:     []string{"get", "list"},
			APIGroups: []string{""},
			Resources: []string{"pods"},
		}},
	}, metav1.CreateOptions{})
	_, _ = client.RbacV1().RoleBindings("default").Create(s.T().Context(), &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-pods-list"},
		Subjects:   []rbacv1.Subject{{Kind: "User", Name: envTestUser.Name}},
		RoleRef:    rbacv1.RoleRef{Kind: "Role", Name: r.Name},
	}, metav1.CreateOptions{})
	// Deny cluster by removing cluster rule
	_ = client.RbacV1().ClusterRoles().Delete(s.T().Context(), "allow-all", metav1.DeleteOptions{})
	s.Run("pods_list returns pods list for default namespace only", func() {
		toolResult, err := s.CallTool("pods_list", map[string]interface{}{})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed %v", toolResult.Content)
		})
		var decoded []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decoded)
		s.Run("has yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
		})
		s.Run("returns at least 1 item", func() {
			s.GreaterOrEqualf(len(decoded), 1, "invalid pods count, expected at least 1, got %v", len(decoded))
		})
		s.Run("all pods are in default namespace", func() {
			for _, pod := range decoded {
				s.Equalf("default", pod.GetNamespace(), "all pods should be in default namespace, got pod %s in namespace %s", pod.GetName(), pod.GetNamespace())
			}
		})
		s.Run("includes a-pod-in-default", func() {
			found := false
			for _, pod := range decoded {
				if pod.GetName() == "a-pod-in-default" {
					found = true
					break
				}
			}
			s.Truef(found, "expected to find pod a-pod-in-default")
		})
	})
}

func (s *PodsSuite) TestPodsListInNamespace() {
	s.InitMcpClient()
	s.Run("pods_list_in_namespace with nil namespace returns error", func() {
		toolResult, _ := s.CallTool("pods_list_in_namespace", map[string]interface{}{})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf("failed to list pods in namespace, missing argument namespace", toolResult.Content[0].(mcp.TextContent).Text,
			"invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})
	s.Run("pods_list_in_namespace(namespace=ns-1) returns pods list", func() {
		toolResult, err := s.CallTool("pods_list_in_namespace", map[string]interface{}{
			"namespace": "ns-1",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		var decoded []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decoded)
		s.Run("has yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
		})
		s.Run("returns 1 item", func() {
			s.Lenf(decoded, 1, "invalid pods count, expected 1, got %v", len(decoded))
		})
		s.Run("returns pod in ns-1", func() {
			s.Equalf("a-pod-in-ns-1", decoded[0].GetName(), "invalid pod name, expected a-pod-in-ns-1, got %v", decoded[0].GetName())
			s.Equalf("ns-1", decoded[0].GetNamespace(), "invalid pod namespace, expected ns-1, got %v", decoded[0].GetNamespace())
		})
		s.Run("omits managed fields", func() {
			s.Nilf(decoded[0].GetManagedFields(), "managed fields should be omitted, got %v", decoded[0].GetManagedFields())
		})
	})
}

func (s *PodsSuite) TestPodsListDenied() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		denied_resources = [ { version = "v1", kind = "Pod" } ]
	`), s.Cfg), "Expected to parse denied resources config")
	s.InitMcpClient()
	s.Run("pods_list (denied)", func() {
		podsList, err := s.CallTool("pods_list", map[string]interface{}{})
		s.Run("has error", func() {
			s.Truef(podsList.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			msg := podsList.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			expectedMessage := "failed to list pods in all namespaces:(.+:)? resource not allowed: /v1, Kind=Pod"
			s.Regexpf(expectedMessage, msg,
				"expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
	s.Run("pods_list_in_namespace (denied)", func() {
		podsListInNamespace, err := s.CallTool("pods_list_in_namespace", map[string]interface{}{"namespace": "ns-1"})
		s.Run("has error", func() {
			s.Truef(podsListInNamespace.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			msg := podsListInNamespace.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			expectedMessage := "failed to list pods in namespace ns-1:(.+:)? resource not allowed: /v1, Kind=Pod"
			s.Regexpf(expectedMessage, msg,
				"expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
}

func (s *PodsSuite) TestPodsListForbidden() {
	s.InitMcpClient()
	defer restoreAuth(s.T().Context())
	client := kubernetes.NewForConfigOrDie(envTestRestConfig)
	// Remove all permissions - user will have forbidden access
	_ = client.RbacV1().ClusterRoles().Delete(s.T().Context(), "allow-all", metav1.DeleteOptions{})

	s.Run("pods_list (forbidden)", func() {
		capture := s.StartCapturingLogNotifications()
		toolResult, _ := s.CallTool("pods_list", map[string]interface{}{})
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

func (s *PodsSuite) TestPodsListAsTable() {
	s.Cfg.ListOutput = "table"
	s.InitMcpClient()
	s.Run("pods_list (list_output=table)", func() {
		podsList, err := s.CallTool("pods_list", map[string]interface{}{})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(podsList.IsError, "call tool failed")
		})
		s.Require().NotNil(podsList, "Expected tool result from call")
		outPodsList := podsList.Content[0].(mcp.TextContent).Text
		s.Run("returns table with header and rows", func() {
			lines := strings.Count(outPodsList, "\n")
			s.GreaterOrEqualf(lines, 3, "invalid line count, expected at least 3 (1 header, 2+ rows), got %v", lines)
		})
		s.Run("returns column headers", func() {
			expectedHeaders := "NAMESPACE\\s+APIVERSION\\s+KIND\\s+NAME\\s+READY\\s+STATUS\\s+RESTARTS\\s+AGE\\s+IP\\s+NODE\\s+NOMINATED NODE\\s+READINESS GATES\\s+LABELS"
			m, e := regexp.MatchString(expectedHeaders, outPodsList)
			s.Truef(m, "Expected headers '%s' not found in output:\n%s", expectedHeaders, outPodsList)
			s.NoErrorf(e, "Error matching headers regex: %v", e)
		})
		s.Run("returns formatted row for a-pod-in-ns-1", func() {
			expectedRow := "(?<namespace>ns-1)\\s+" +
				"(?<apiVersion>v1)\\s+" +
				"(?<kind>Pod)\\s+" +
				"(?<name>a-pod-in-ns-1)\\s+" +
				"(?<ready>0\\/1)\\s+" +
				"(?<status>Pending)\\s+" +
				"(?<restarts>0)\\s+" +
				"(?<age>(\\d+m)?(\\d+s)?)\\s+" +
				"(?<ip><none>)\\s+" +
				"(?<node><none>)\\s+" +
				"(?<nominated_node><none>)\\s+" +
				"(?<readiness_gates><none>)\\s+" +
				"(?<labels><none>)"
			m, e := regexp.MatchString(expectedRow, outPodsList)
			s.Truef(m, "Expected row '%s' not found in output:\n%s", expectedRow, outPodsList)
			s.NoErrorf(e, "Error matching a-pod-in-ns-1 regex: %v", e)
		})
		s.Run("returns formatted row for a-pod-in-default", func() {
			expectedRow := "(?<namespace>default)\\s+" +
				"(?<apiVersion>v1)\\s+" +
				"(?<kind>Pod)\\s+" +
				"(?<name>a-pod-in-default)\\s+" +
				"(?<ready>0\\/1)\\s+" +
				"(?<status>Pending)\\s+" +
				"(?<restarts>0)\\s+" +
				"(?<age>(\\d+m)?(\\d+s)?)\\s+" +
				"(?<ip><none>)\\s+" +
				"(?<node><none>)\\s+" +
				"(?<nominated_node><none>)\\s+" +
				"(?<readiness_gates><none>)\\s+" +
				"(?<labels>app=nginx)"
			m, e := regexp.MatchString(expectedRow, outPodsList)
			s.Truef(m, "Expected row '%s' not found in output:\n%s", expectedRow, outPodsList)
			s.NoErrorf(e, "Error matching a-pod-in-default regex: %v", e)
		})
	})
	s.Run("pods_list_in_namespace (list_output=table)", func() {
		podsListInNamespace, err := s.CallTool("pods_list_in_namespace", map[string]interface{}{
			"namespace": "ns-1",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(podsListInNamespace.IsError, "call tool failed")
		})
		s.Require().NotNil(podsListInNamespace, "Expected tool result from call")
		outPodsListInNamespace := podsListInNamespace.Content[0].(mcp.TextContent).Text
		s.Run("returns table with header and row", func() {
			lines := strings.Count(outPodsListInNamespace, "\n")
			s.GreaterOrEqualf(lines, 1, "invalid line count, expected at least 1 (1 header, 1+ rows), got %v", lines)
		})
		s.Run("returns column headers", func() {
			expectedHeaders := "NAMESPACE\\s+APIVERSION\\s+KIND\\s+NAME\\s+READY\\s+STATUS\\s+RESTARTS\\s+AGE\\s+IP\\s+NODE\\s+NOMINATED NODE\\s+READINESS GATES\\s+LABELS"
			m, e := regexp.MatchString(expectedHeaders, outPodsListInNamespace)
			s.Truef(m, "Expected headers '%s' not found in output:\n%s", expectedHeaders, outPodsListInNamespace)
			s.NoErrorf(e, "Error matching headers regex: %v", e)
		})
		s.Run("returns formatted row", func() {
			expectedRow := "(?<namespace>ns-1)\\s+" +
				"(?<apiVersion>v1)\\s+" +
				"(?<kind>Pod)\\s+" +
				"(?<name>a-pod-in-ns-1)\\s+" +
				"(?<ready>0\\/1)\\s+" +
				"(?<status>Pending)\\s+" +
				"(?<restarts>0)\\s+" +
				"(?<age>(\\d+m)?(\\d+s)?)\\s+" +
				"(?<ip><none>)\\s+" +
				"(?<node><none>)\\s+" +
				"(?<nominated_node><none>)\\s+" +
				"(?<readiness_gates><none>)\\s+" +
				"(?<labels><none>)"
			m, e := regexp.MatchString(expectedRow, outPodsListInNamespace)
			s.Truef(m, "Expected row '%s' not found in output:\n%s", expectedRow, outPodsListInNamespace)
			s.NoErrorf(e, "Error matching formatted row regex: %v", e)
		})
	})
}

func (s *PodsSuite) TestPodsGet() {
	s.InitMcpClient()
	s.Run("pods_get with nil name returns error", func() {
		toolResult, _ := s.CallTool("pods_get", map[string]interface{}{})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf("failed to get pod, missing argument name", toolResult.Content[0].(mcp.TextContent).Text, "invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})
	s.Run("pods_get(name=not-found) with not found name", func() {
		capture := s.StartCapturingLogNotifications()
		toolResult, _ := s.CallTool("pods_get", map[string]interface{}{"name": "not-found"})
		s.Run("returns error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
			s.Equalf("failed to get pod not-found in namespace : pods \"not-found\" not found", toolResult.Content[0].(mcp.TextContent).Text, "invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
		})
		s.Run("sends log notification", func() {
			logNotification := capture.RequireLogNotification(s.T(), 2*time.Second)
			s.Equal("info", logNotification.Level, "not found errors should log at info level")
			s.Contains(logNotification.Data, "Resource not found", "log message should indicate resource not found")
		})
	})
	s.Run("pods_get(name=a-pod-in-default, namespace=nil), uses configured namespace", func() {
		podsGetNilNamespace, err := s.CallTool("pods_get", map[string]interface{}{
			"name": "a-pod-in-default",
		})
		s.Run("returns pod", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(podsGetNilNamespace.IsError, "call tool failed")
		})
		var decodedNilNamespace unstructured.Unstructured
		err = yaml.Unmarshal([]byte(podsGetNilNamespace.Content[0].(mcp.TextContent).Text), &decodedNilNamespace)
		s.Run("has yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
		})
		s.Run("returns pod in default", func() {
			s.Equalf("a-pod-in-default", decodedNilNamespace.GetName(), "invalid pod name, expected a-pod-in-default, got %v", decodedNilNamespace.GetName())
			s.Equalf("default", decodedNilNamespace.GetNamespace(), "invalid pod namespace, expected default, got %v", decodedNilNamespace.GetNamespace())
		})
		s.Run("omits managed fields", func() {
			s.Nilf(decodedNilNamespace.GetManagedFields(), "managed fields should be omitted, got %v", decodedNilNamespace.GetManagedFields())
		})
	})
	s.Run("pods_get(name=a-pod-in-default, namespace=ns-1)", func() {
		podsGetInNamespace, err := s.CallTool("pods_get", map[string]interface{}{
			"namespace": "ns-1",
			"name":      "a-pod-in-ns-1",
		})
		s.Run("returns pod", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(podsGetInNamespace.IsError, "call tool failed")
		})
		var decodedInNamespace unstructured.Unstructured
		err = yaml.Unmarshal([]byte(podsGetInNamespace.Content[0].(mcp.TextContent).Text), &decodedInNamespace)
		s.Run("has yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
		})
		s.Run("returns pod in ns-1", func() {
			s.Equalf("a-pod-in-ns-1", decodedInNamespace.GetName(), "invalid pod name, expected a-pod-in-ns-1, got %v", decodedInNamespace.GetName())
			s.Equalf("ns-1", decodedInNamespace.GetNamespace(), "invalid pod namespace, expected ns-1, got %v", decodedInNamespace.GetNamespace())
		})
	})
}

func (s *PodsSuite) TestPodsGetDenied() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		denied_resources = [ { version = "v1", kind = "Pod" } ]
	`), s.Cfg), "Expected to parse denied resources config")
	s.InitMcpClient()
	s.Run("pods_get (denied)", func() {
		podsGet, err := s.CallTool("pods_get", map[string]interface{}{"name": "a-pod-in-default"})
		s.Run("has error", func() {
			s.Truef(podsGet.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			msg := podsGet.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			expectedMessage := "failed to get pod a-pod-in-default in namespace :(.+:)? resource not allowed: /v1, Kind=Pod"
			s.Regexpf(expectedMessage, msg,
				"expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
}

func (s *PodsSuite) TestPodsDelete() {
	s.InitMcpClient()
	s.Run("pods_delete with nil name returns error", func() {
		toolResult, _ := s.CallTool("pods_delete", map[string]interface{}{})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf("failed to delete pod, missing argument name", toolResult.Content[0].(mcp.TextContent).Text, "invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})
	s.Run("pods_delete(name=not-found) with not found name", func() {
		capture := s.StartCapturingLogNotifications()
		toolResult, _ := s.CallTool("pods_delete", map[string]interface{}{"name": "not-found"})
		s.Run("returns error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
			s.Equalf("failed to delete pod not-found in namespace : pods \"not-found\" not found", toolResult.Content[0].(mcp.TextContent).Text, "invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
		})
		s.Run("sends log notification", func() {
			logNotification := capture.RequireLogNotification(s.T(), 2*time.Second)
			s.Equal("info", logNotification.Level, "not found errors should log at info level")
			s.Contains(logNotification.Data, "Resource not found", "log message should indicate resource not found")
		})
	})
	s.Run("pods_delete(name=a-pod-to-delete, namespace=nil), uses configured namespace", func() {
		kc := kubernetes.NewForConfigOrDie(envTestRestConfig)
		_, _ = kc.CoreV1().Pods("default").Create(s.T().Context(), &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "a-pod-to-delete"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
		}, metav1.CreateOptions{})
		podsDeleteNilNamespace, err := s.CallTool("pods_delete", map[string]interface{}{
			"name": "a-pod-to-delete",
		})
		s.Run("returns success", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(podsDeleteNilNamespace.IsError, "call tool failed")
			s.Equalf("Pod deleted successfully", podsDeleteNilNamespace.Content[0].(mcp.TextContent).Text, "invalid tool result content, got %v", podsDeleteNilNamespace.Content[0].(mcp.TextContent).Text)
		})
		s.Run("deletes Pod", func() {
			p, pErr := kc.CoreV1().Pods("default").Get(s.T().Context(), "a-pod-to-delete", metav1.GetOptions{})
			s.Truef(pErr != nil || p == nil || p.DeletionTimestamp != nil, "Pod not deleted")
		})
	})
	s.Run("pods_delete(name=a-pod-to-delete-in-ns-1, namespace=ns-1)", func() {
		kc := kubernetes.NewForConfigOrDie(envTestRestConfig)
		_, _ = kc.CoreV1().Pods("ns-1").Create(s.T().Context(), &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "a-pod-to-delete-in-ns-1"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
		}, metav1.CreateOptions{})
		podsDeleteInNamespace, err := s.CallTool("pods_delete", map[string]interface{}{
			"namespace": "ns-1",
			"name":      "a-pod-to-delete-in-ns-1",
		})
		s.Run("returns success", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(podsDeleteInNamespace.IsError, "call tool failed")
			s.Equalf("Pod deleted successfully", podsDeleteInNamespace.Content[0].(mcp.TextContent).Text, "invalid tool result content, got %v", podsDeleteInNamespace.Content[0].(mcp.TextContent).Text)
		})
		s.Run("deletes Pod", func() {
			p, pErr := kc.CoreV1().Pods("ns-1").Get(s.T().Context(), "a-pod-to-delete-in-ns-1", metav1.GetOptions{})
			s.Truef(pErr != nil || p == nil || p.DeletionTimestamp != nil, "Pod not deleted")
		})
	})
	s.Run("pods_delete(name=a-managed-pod-to-delete, namespace=ns-1) with managed pod", func() {
		kc := kubernetes.NewForConfigOrDie(envTestRestConfig)
		managedLabels := map[string]string{
			"app.kubernetes.io/managed-by": "kubernetes-mcp-server",
			"app.kubernetes.io/name":       "a-manged-pod-to-delete",
		}
		_, _ = kc.CoreV1().Pods("default").Create(s.T().Context(), &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "a-managed-pod-to-delete", Labels: managedLabels},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
		}, metav1.CreateOptions{})
		_, _ = kc.CoreV1().Services("default").Create(s.T().Context(), &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "a-managed-service-to-delete", Labels: managedLabels},
			Spec:       corev1.ServiceSpec{Selector: managedLabels, Ports: []corev1.ServicePort{{Port: 80}}},
		}, metav1.CreateOptions{})
		podsDeleteManaged, err := s.CallTool("pods_delete", map[string]interface{}{
			"name": "a-managed-pod-to-delete",
		})
		s.Run("returns success", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(podsDeleteManaged.IsError, "call tool failed")
			s.Equalf("Pod deleted successfully", podsDeleteManaged.Content[0].(mcp.TextContent).Text, "invalid tool result content, got %v", podsDeleteManaged.Content[0].(mcp.TextContent).Text)
		})
		s.Run("deletes Pod and Service", func() {
			p, pErr := kc.CoreV1().Pods("default").Get(s.T().Context(), "a-managed-pod-to-delete", metav1.GetOptions{})
			s.Truef(pErr != nil || p == nil || p.DeletionTimestamp != nil, "Pod not deleted")
			svc, sErr := kc.CoreV1().Services("default").Get(s.T().Context(), "a-managed-service-to-delete", metav1.GetOptions{})
			s.Truef(sErr != nil || svc == nil || svc.DeletionTimestamp != nil, "Service not deleted")
		})
	})
}

func (s *PodsSuite) TestPodsDeleteDenied() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		denied_resources = [ { version = "v1", kind = "Pod" } ]
	`), s.Cfg), "Expected to parse denied resources config")
	s.InitMcpClient()
	s.Run("pods_delete (denied)", func() {
		podsDelete, err := s.CallTool("pods_delete", map[string]interface{}{"name": "a-pod-in-default"})
		s.Run("has error", func() {
			s.Truef(podsDelete.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			msg := podsDelete.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			expectedMessage := "failed to delete pod a-pod-in-default in namespace :(.+:)? resource not allowed: /v1, Kind=Pod"
			s.Regexpf(expectedMessage, msg,
				"expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
}

func (s *PodsSuite) TestPodsDeleteInOpenShift() {
	s.Require().NoError(EnvTestInOpenShift(s.T().Context()), "Expected to configure test for OpenShift")
	s.T().Cleanup(func() {
		s.Require().NoError(EnvTestInOpenShiftClear(s.T().Context()), "Expected to clear OpenShift test configuration")
	})
	s.InitMcpClient()

	s.Run("pods_delete with managed pod in OpenShift", func() {
		managedLabels := map[string]string{
			"app.kubernetes.io/managed-by": "kubernetes-mcp-server",
			"app.kubernetes.io/name":       "a-manged-pod-to-delete",
		}
		kc := kubernetes.NewForConfigOrDie(envTestRestConfig)
		_, _ = kc.CoreV1().Pods("default").Create(s.T().Context(), &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "a-managed-pod-to-delete-in-openshift", Labels: managedLabels},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
		}, metav1.CreateOptions{})
		dynamicClient := dynamic.NewForConfigOrDie(envTestRestConfig)
		_, _ = dynamicClient.Resource(schema.GroupVersionResource{Group: "route.openshift.io", Version: "v1", Resource: "routes"}).
			Namespace("default").Create(s.T().Context(), &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "route.openshift.io/v1",
			"kind":       "Route",
			"metadata": map[string]interface{}{
				"name":   "a-managed-route-to-delete",
				"labels": managedLabels,
			},
		}}, metav1.CreateOptions{})
		podsDeleteManagedOpenShift, err := s.CallTool("pods_delete", map[string]interface{}{
			"name": "a-managed-pod-to-delete-in-openshift",
		})
		s.Run("returns success", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(podsDeleteManagedOpenShift.IsError, "call tool failed")
			s.Equalf("Pod deleted successfully", podsDeleteManagedOpenShift.Content[0].(mcp.TextContent).Text,
				"invalid tool result content, got %v", podsDeleteManagedOpenShift.Content[0].(mcp.TextContent).Text)
		})
		s.Run("deletes Pod and Route", func() {
			p, pErr := kc.CoreV1().Pods("default").Get(s.T().Context(), "a-managed-pod-to-delete-in-openshift", metav1.GetOptions{})
			s.False(pErr == nil && p != nil && p.DeletionTimestamp == nil, "Pod not deleted")
			r, rErr := dynamicClient.
				Resource(schema.GroupVersionResource{Group: "route.openshift.io", Version: "v1", Resource: "routes"}).
				Namespace("default").Get(s.T().Context(), "a-managed-route-to-delete", metav1.GetOptions{})
			s.False(rErr == nil && r != nil && r.GetDeletionTimestamp() == nil, "Route not deleted")
		})
	})
}

func (s *PodsSuite) TestPodsLog() {
	s.InitMcpClient()
	s.Run("pods_log with nil name returns error", func() {
		toolResult, _ := s.CallTool("pods_log", map[string]interface{}{})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf("failed to get pod log, missing argument name", toolResult.Content[0].(mcp.TextContent).Text, "invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})
	s.Run("pods_log with not found name returns error", func() {
		toolResult, _ := s.CallTool("pods_log", map[string]interface{}{"name": "not-found"})
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf("failed to get pod not-found log in namespace : pods \"not-found\" not found", toolResult.Content[0].(mcp.TextContent).Text, "invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})
	s.Run("pods_log(name=a-pod-in-default, namespace=nil), uses configured namespace", func() {
		podsLogNilNamespace, err := s.CallTool("pods_log", map[string]interface{}{
			"name": "a-pod-in-default",
		})
		s.Nilf(err, "call tool failed %v", err)
		s.Falsef(podsLogNilNamespace.IsError, "call tool failed")
	})
	s.Run("pods_log(name=a-pod-in-ns-1, namespace=ns-1)", func() {
		podsLogInNamespace, err := s.CallTool("pods_log", map[string]interface{}{
			"namespace": "ns-1",
			"name":      "a-pod-in-ns-1",
		})
		s.Nilf(err, "call tool failed %v", err)
		s.Falsef(podsLogInNamespace.IsError, "call tool failed")
	})
	s.Run("pods_log(name=a-pod-in-ns-1, namespace=ns-1, container=nginx)", func() {
		podsContainerLogInNamespace, err := s.CallTool("pods_log", map[string]interface{}{
			"namespace": "ns-1",
			"name":      "a-pod-in-ns-1",
			"container": "nginx",
		})
		s.Nilf(err, "call tool failed %v", err)
		s.Falsef(podsContainerLogInNamespace.IsError, "call tool failed")
	})
	s.Run("with non existing container returns error", func() {
		toolResult, err := s.CallTool("pods_log", map[string]interface{}{
			"namespace": "ns-1",
			"name":      "a-pod-in-ns-1",
			"container": "a-not-existing-container",
		})
		s.Nilf(err, "call tool should not return error object")
		s.Truef(toolResult.IsError, "call tool should fail")
		s.Equalf("failed to get pod a-pod-in-ns-1 log in namespace ns-1: container a-not-existing-container is not valid for pod a-pod-in-ns-1", toolResult.Content[0].(mcp.TextContent).Text, "invalid error message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})
	s.Run("pods_log(previous=true) returns previous pod log", func() {
		podsPreviousLogInNamespace, err := s.CallTool("pods_log", map[string]interface{}{
			"namespace": "ns-1",
			"name":      "a-pod-in-ns-1",
			"previous":  true,
		})
		s.Nilf(err, "call tool failed %v", err)
		s.Falsef(podsPreviousLogInNamespace.IsError, "call tool failed")
	})
	s.Run("pods_log(previous=false) returns current pod log", func() {
		podsPreviousLogFalse, err := s.CallTool("pods_log", map[string]interface{}{
			"namespace": "ns-1",
			"name":      "a-pod-in-ns-1",
			"previous":  false,
		})
		s.Nilf(err, "call tool failed %v", err)
		s.Falsef(podsPreviousLogFalse.IsError, "call tool failed")
	})
	s.Run("pods_log(tail=50) returns pod log", func() {
		podsTailLines, err := s.CallTool("pods_log", map[string]interface{}{
			"namespace": "ns-1",
			"name":      "a-pod-in-ns-1",
			"tail":      50,
		})
		s.Nilf(err, "call tool failed %v", err)
		s.Falsef(podsTailLines.IsError, "call tool failed")
	})
	s.Run("with invalid tail returns error", func() {
		podsInvalidTailLines, _ := s.CallTool("pods_log", map[string]interface{}{
			"namespace": "ns-1",
			"name":      "a-pod-in-ns-1",
			"tail":      "invalid",
		})
		s.Truef(podsInvalidTailLines.IsError, "call tool should fail")
		expectedErrorMsg := "failed to parse tail parameter: expected integer"
		errMsg := podsInvalidTailLines.Content[0].(mcp.TextContent).Text
		s.Containsf(errMsg, expectedErrorMsg, "unexpected error message, expected to contain '%s', got '%s'", expectedErrorMsg, errMsg)
	})
}

func (s *PodsSuite) TestPodsLogDenied() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		denied_resources = [ { version = "v1", kind = "Pod" } ]
	`), s.Cfg), "Expected to parse denied resources config")
	s.InitMcpClient()
	s.Run("pods_log (denied)", func() {
		podsLog, err := s.CallTool("pods_log", map[string]interface{}{"name": "a-pod-in-default"})
		s.Run("has error", func() {
			s.Truef(podsLog.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			msg := podsLog.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			expectedMessage := "failed to get pod a-pod-in-default log in namespace :(.+:)? resource not allowed: /v1, Kind=Pod"
			s.Regexpf(expectedMessage, msg,
				"expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
}

func (s *PodsSuite) TestPodsListWithLabelSelector() {
	s.InitMcpClient()
	kc := kubernetes.NewForConfigOrDie(envTestRestConfig)
	// Create pods with labels
	_, _ = kc.CoreV1().Pods("default").Create(s.T().Context(), &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "pod-with-labels",
			Labels: map[string]string{"app": "test", "env": "dev"},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
	}, metav1.CreateOptions{})
	_, _ = kc.CoreV1().Pods("ns-1").Create(s.T().Context(), &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "another-pod-with-labels",
			Labels: map[string]string{"app": "test", "env": "prod"},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
	}, metav1.CreateOptions{})

	s.Run("pods_list(labelSelector=app=test) returns filtered pods from configured namespace", func() {
		toolResult, err := s.CallTool("pods_list", map[string]interface{}{
			"labelSelector": "app=test",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		var decoded []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decoded)
		s.Run("has yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
		})
		s.Run("returns 2 pods", func() {
			s.Lenf(decoded, 2, "invalid pods count, expected 2, got %v", len(decoded))
		})
	})

	s.Run("pods_list_in_namespace(labelSelector=env=prod, namespace=ns-1) returns filtered pods", func() {
		toolResult, err := s.CallTool("pods_list_in_namespace", map[string]interface{}{
			"namespace":     "ns-1",
			"labelSelector": "env=prod",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		var decoded []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decoded)
		s.Run("has yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
		})
		s.Run("returns 1 pod", func() {
			s.Lenf(decoded, 1, "invalid pods count, expected 1, got %v", len(decoded))
		})
		s.Run("returns another-pod-with-labels", func() {
			s.Equalf("another-pod-with-labels", decoded[0].GetName(), "invalid pod name, expected another-pod-with-labels, got %v", decoded[0].GetName())
		})
	})

	s.Run("pods_list(labelSelector=app=test,env=prod) with multiple label selectors returns filtered pods", func() {
		toolResult, err := s.CallTool("pods_list", map[string]interface{}{
			"labelSelector": "app=test,env=prod",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		var decoded []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decoded)
		s.Run("has yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
		})
		s.Run("returns 1 pod", func() {
			s.Lenf(decoded, 1, "invalid pods count, expected 1, got %v", len(decoded))
		})
		s.Run("returns another-pod-with-labels", func() {
			s.Equalf("another-pod-with-labels", decoded[0].GetName(), "invalid pod name, expected another-pod-with-labels, got %v", decoded[0].GetName())
		})
	})
}

func (s *PodsSuite) TestPodsListWithFieldSelector() {
	s.InitMcpClient()
	kc := kubernetes.NewForConfigOrDie(envTestRestConfig)
	// Create multiple pods for field selector testing to verify filtering excludes unwanted pods
	_, _ = kc.CoreV1().Pods("default").Create(s.T().Context(), &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "field-selector-target",
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
	}, metav1.CreateOptions{})
	_, _ = kc.CoreV1().Pods("default").Create(s.T().Context(), &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "field-selector-excluded",
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
	}, metav1.CreateOptions{})

	s.Run("pods_list(fieldSelector=metadata.name=field-selector-target) returns only matching pod", func() {
		toolResult, err := s.CallTool("pods_list", map[string]interface{}{
			"fieldSelector": "metadata.name=field-selector-target",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		var decoded []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decoded)
		s.Run("has yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
		})
		s.Run("returns exactly 1 pod", func() {
			s.Lenf(decoded, 1, "expected exactly 1 pod, got %v", len(decoded))
		})
		s.Run("returns field-selector-target", func() {
			s.Equalf("field-selector-target", decoded[0].GetName(), "expected field-selector-target, got %v", decoded[0].GetName())
		})
		s.Run("excludes field-selector-excluded", func() {
			for _, pod := range decoded {
				s.NotEqualf("field-selector-excluded", pod.GetName(), "field-selector-excluded should have been filtered out")
			}
		})
	})

	s.Run("pods_list_in_namespace(fieldSelector=metadata.name=field-selector-target, namespace=default) returns only matching pod", func() {
		toolResult, err := s.CallTool("pods_list_in_namespace", map[string]interface{}{
			"namespace":     "default",
			"fieldSelector": "metadata.name=field-selector-target",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		var decoded []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decoded)
		s.Run("has yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
		})
		s.Run("returns exactly 1 pod", func() {
			s.Lenf(decoded, 1, "expected exactly 1 pod, got %v", len(decoded))
		})
		s.Run("returns field-selector-target", func() {
			s.Equalf("field-selector-target", decoded[0].GetName(), "expected field-selector-target, got %v", decoded[0].GetName())
		})
		s.Run("excludes field-selector-excluded", func() {
			for _, pod := range decoded {
				s.NotEqualf("field-selector-excluded", pod.GetName(), "field-selector-excluded should have been filtered out")
			}
		})
	})

	s.Run("pods_list with combined labelSelector and fieldSelector returns only matching pod", func() {
		// Create pods with specific labels for combined filtering
		_, _ = kc.CoreV1().Pods("default").Create(s.T().Context(), &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "combined-selector-target",
				Labels: map[string]string{"env": "test-field-combined"},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
		}, metav1.CreateOptions{})
		_, _ = kc.CoreV1().Pods("default").Create(s.T().Context(), &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "combined-selector-same-label",
				Labels: map[string]string{"env": "test-field-combined"},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
		}, metav1.CreateOptions{})

		toolResult, err := s.CallTool("pods_list", map[string]interface{}{
			"labelSelector": "env=test-field-combined",
			"fieldSelector": "metadata.name=combined-selector-target",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		var decoded []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decoded)
		s.Run("has yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
		})
		s.Run("returns exactly 1 pod", func() {
			s.Lenf(decoded, 1, "expected exactly 1 pod, got %v", len(decoded))
		})
		s.Run("returns combined-selector-target", func() {
			s.Equalf("combined-selector-target", decoded[0].GetName(), "expected combined-selector-target, got %v", decoded[0].GetName())
		})
		s.Run("excludes combined-selector-same-label despite matching label", func() {
			for _, pod := range decoded {
				s.NotEqualf("combined-selector-same-label", pod.GetName(), "combined-selector-same-label should have been filtered out by fieldSelector")
			}
		})
	})
}

func TestPods(t *testing.T) {
	suite.Run(t, new(PodsSuite))
}
