package mcp

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
	"golang.org/x/sync/errgroup"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var argocdApis = []schema.GroupVersionResource{
	{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"},
	{Group: "argoproj.io", Version: "v1alpha1", Resource: "appprojects"},
	{Group: "argoproj.io", Version: "v1beta1", Resource: "argocds"},
}

type ArgocdSuite struct {
	BaseMcpSuite
}

func (s *ArgocdSuite) SetupSuite() {
	ctx := s.T().Context()
	tasks, _ := errgroup.WithContext(ctx)
	for _, api := range argocdApis {
		gvr := api
		tasks.Go(func() error { return EnvTestEnableCRD(ctx, gvr.Group, gvr.Version, gvr.Resource) })
	}
	s.Require().NoError(tasks.Wait())
}

func (s *ArgocdSuite) TearDownSuite() {
	tasks, _ := errgroup.WithContext(s.T().Context())
	for _, api := range argocdApis {
		gvr := api
		tasks.Go(func() error { return EnvTestDisableCRD(s.T().Context(), gvr.Group, gvr.Version, gvr.Resource) })
	}
	s.Require().NoError(tasks.Wait())
}

func (s *ArgocdSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.Require().NoError(toml.Unmarshal([]byte(`
		toolsets = [ "argocd" ]
	`), s.Cfg), "Expected to parse toolsets config")
	s.InitMcpClient()
}

func (s *ArgocdSuite) TestApplicationList() {
	dynamicClient := dynamic.NewForConfigOrDie(envTestRestConfig)
	appResource := schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}

	s.Run("empty list", func() {
		toolResult, err := s.CallTool("argocd_application_list", map[string]interface{}{
			"namespace": "default",
		})
		s.Require().NoError(err)
		s.False(toolResult.IsError, "expected no error")
	})

	s.Run("list with items", func() {
		app := newUnstructuredApplication("test-app-list", "default", map[string]interface{}{"env": "test"})
		_, err := dynamicClient.Resource(appResource).Namespace("default").Create(s.T().Context(), app, metav1.CreateOptions{})
		s.Require().NoError(err)

		toolResult, err := s.CallTool("argocd_application_list", map[string]interface{}{
			"namespace": "default",
		})
		s.Require().NoError(err)
		s.False(toolResult.IsError, "expected no error")
		s.Contains(toolResult.Content[0].(*mcp.TextContent).Text, "test-app-list")

		_ = dynamicClient.Resource(appResource).Namespace("default").Delete(s.T().Context(), "test-app-list", metav1.DeleteOptions{})
	})

	s.Run("list with labelSelector", func() {
		appA := newUnstructuredApplication("app-labeled-a", "default", map[string]interface{}{"team": "alpha"})
		appB := newUnstructuredApplication("app-labeled-b", "default", map[string]interface{}{"team": "beta"})
		_, err := dynamicClient.Resource(appResource).Namespace("default").Create(s.T().Context(), appA, metav1.CreateOptions{})
		s.Require().NoError(err)
		_, err = dynamicClient.Resource(appResource).Namespace("default").Create(s.T().Context(), appB, metav1.CreateOptions{})
		s.Require().NoError(err)

		toolResult, err := s.CallTool("argocd_application_list", map[string]interface{}{
			"namespace":     "default",
			"labelSelector": "team=alpha",
		})
		s.Require().NoError(err)
		s.False(toolResult.IsError, "expected no error")
		text := toolResult.Content[0].(*mcp.TextContent).Text
		s.Contains(text, "app-labeled-a")
		s.False(strings.Contains(text, "app-labeled-b"), "expected app-labeled-b to be filtered out")

		_ = dynamicClient.Resource(appResource).Namespace("default").Delete(s.T().Context(), "app-labeled-a", metav1.DeleteOptions{})
		_ = dynamicClient.Resource(appResource).Namespace("default").Delete(s.T().Context(), "app-labeled-b", metav1.DeleteOptions{})
	})
}

func (s *ArgocdSuite) TestApplicationGet() {
	dynamicClient := dynamic.NewForConfigOrDie(envTestRestConfig)
	appResource := schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}

	s.Run("get existing application", func() {
		app := newUnstructuredApplication("test-app-get", "default", nil)
		_, err := dynamicClient.Resource(appResource).Namespace("default").Create(s.T().Context(), app, metav1.CreateOptions{})
		s.Require().NoError(err)

		toolResult, err := s.CallTool("argocd_application_get", map[string]interface{}{
			"name":      "test-app-get",
			"namespace": "default",
		})
		s.Require().NoError(err)
		s.False(toolResult.IsError, "expected no error")
		text := toolResult.Content[0].(*mcp.TextContent).Text
		s.Contains(text, "# Application: test-app-get")
		s.Contains(text, "Source: https://github.com/example/repo (path: manifests, revision: HEAD)")
		s.Contains(text, "Destination: https://kubernetes.default.svc / default")
		s.Contains(text, "apiVersion: argoproj.io/v1alpha1")

		_ = dynamicClient.Resource(appResource).Namespace("default").Delete(s.T().Context(), "test-app-get", metav1.DeleteOptions{})
	})

	s.Run("get non-existent application", func() {
		toolResult, err := s.CallTool("argocd_application_get", map[string]interface{}{
			"name":      "non-existent-app",
			"namespace": "default",
		})
		s.Require().NoError(err)
		s.True(toolResult.IsError, "expected error for non-existent application")
		s.Contains(toolResult.Content[0].(*mcp.TextContent).Text, "failed to get Application")
	})
}

func (s *ArgocdSuite) TestAppProjectList() {
	dynamicClient := dynamic.NewForConfigOrDie(envTestRestConfig)
	projResource := schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "appprojects"}

	s.Run("list with items", func() {
		proj := newUnstructuredAppProject("test-project", "default", nil)
		_, err := dynamicClient.Resource(projResource).Namespace("default").Create(s.T().Context(), proj, metav1.CreateOptions{})
		s.Require().NoError(err)

		toolResult, err := s.CallTool("argocd_appproject_list", map[string]interface{}{
			"namespace": "default",
		})
		s.Require().NoError(err)
		s.False(toolResult.IsError, "expected no error")
		s.Contains(toolResult.Content[0].(*mcp.TextContent).Text, "test-project")

		_ = dynamicClient.Resource(projResource).Namespace("default").Delete(s.T().Context(), "test-project", metav1.DeleteOptions{})
	})
}

func (s *ArgocdSuite) TestAppProjectGet() {
	dynamicClient := dynamic.NewForConfigOrDie(envTestRestConfig)
	projResource := schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "appprojects"}

	s.Run("get existing appproject", func() {
		proj := newUnstructuredAppProject("test-project-get", "default", nil)
		_, err := dynamicClient.Resource(projResource).Namespace("default").Create(s.T().Context(), proj, metav1.CreateOptions{})
		s.Require().NoError(err)

		toolResult, err := s.CallTool("argocd_appproject_get", map[string]interface{}{
			"name":      "test-project-get",
			"namespace": "default",
		})
		s.Require().NoError(err)
		s.False(toolResult.IsError, "expected no error")
		text := toolResult.Content[0].(*mcp.TextContent).Text
		s.Contains(text, "# AppProject: test-project-get")
		s.Contains(text, "Description: test project")
		s.Contains(text, "apiVersion: argoproj.io/v1alpha1")

		_ = dynamicClient.Resource(projResource).Namespace("default").Delete(s.T().Context(), "test-project-get", metav1.DeleteOptions{})
	})

	s.Run("get non-existent appproject", func() {
		toolResult, err := s.CallTool("argocd_appproject_get", map[string]interface{}{
			"name":      "non-existent-project",
			"namespace": "default",
		})
		s.Require().NoError(err)
		s.True(toolResult.IsError, "expected error for non-existent appproject")
		s.Contains(toolResult.Content[0].(*mcp.TextContent).Text, "failed to get AppProject")
	})
}

func (s *ArgocdSuite) TestInstanceList() {
	dynamicClient := dynamic.NewForConfigOrDie(envTestRestConfig)
	argocdResource := schema.GroupVersionResource{Group: "argoproj.io", Version: "v1beta1", Resource: "argocds"}

	s.Run("list with items", func() {
		instance := newUnstructuredArgoCD("test-argocd", "default", nil)
		_, err := dynamicClient.Resource(argocdResource).Namespace("default").Create(s.T().Context(), instance, metav1.CreateOptions{})
		s.Require().NoError(err)

		toolResult, err := s.CallTool("argocd_instance_list", map[string]interface{}{
			"namespace": "default",
		})
		s.Require().NoError(err)
		s.False(toolResult.IsError, "expected no error")
		s.Contains(toolResult.Content[0].(*mcp.TextContent).Text, "test-argocd")

		_ = dynamicClient.Resource(argocdResource).Namespace("default").Delete(s.T().Context(), "test-argocd", metav1.DeleteOptions{})
	})
}

func (s *ArgocdSuite) TestInstanceGet() {
	dynamicClient := dynamic.NewForConfigOrDie(envTestRestConfig)
	argocdResource := schema.GroupVersionResource{Group: "argoproj.io", Version: "v1beta1", Resource: "argocds"}

	s.Run("get existing instance", func() {
		instance := newUnstructuredArgoCD("test-argocd-get", "default", nil)
		_, err := dynamicClient.Resource(argocdResource).Namespace("default").Create(s.T().Context(), instance, metav1.CreateOptions{})
		s.Require().NoError(err)

		toolResult, err := s.CallTool("argocd_instance_get", map[string]interface{}{
			"name":      "test-argocd-get",
			"namespace": "default",
		})
		s.Require().NoError(err)
		s.False(toolResult.IsError, "expected no error")
		text := toolResult.Content[0].(*mcp.TextContent).Text
		s.Contains(text, "# ArgoCD: test-argocd-get")
		s.Contains(text, "Namespace: default")
		s.Contains(text, "Server: not configured")
		s.Contains(text, "HA: not configured")
		s.Contains(text, "apiVersion: argoproj.io/v1beta1")

		_ = dynamicClient.Resource(argocdResource).Namespace("default").Delete(s.T().Context(), "test-argocd-get", metav1.DeleteOptions{})
	})

	s.Run("get non-existent instance", func() {
		toolResult, err := s.CallTool("argocd_instance_get", map[string]interface{}{
			"name":      "non-existent-argocd",
			"namespace": "default",
		})
		s.Require().NoError(err)
		s.True(toolResult.IsError, "expected error for non-existent instance")
		s.Contains(toolResult.Content[0].(*mcp.TextContent).Text, "failed to get ArgoCD instance")
	})
}

func TestArgocd(t *testing.T) {
	suite.Run(t, new(ArgocdSuite))
}

func newUnstructuredApplication(name, namespace string, labels map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	content := map[string]interface{}{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]interface{}{
			"source": map[string]interface{}{
				"repoURL":        "https://github.com/example/repo",
				"targetRevision": "HEAD",
				"path":           "manifests",
			},
			"destination": map[string]interface{}{
				"server":    "https://kubernetes.default.svc",
				"namespace": "default",
			},
		},
	}
	if labels != nil {
		content["metadata"].(map[string]interface{})["labels"] = labels
	}
	obj.SetUnstructuredContent(content)
	return obj
}

func newUnstructuredAppProject(name, namespace string, labels map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	content := map[string]interface{}{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "AppProject",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]interface{}{
			"description": "test project",
		},
	}
	if labels != nil {
		content["metadata"].(map[string]interface{})["labels"] = labels
	}
	obj.SetUnstructuredContent(content)
	return obj
}

func newUnstructuredArgoCD(name, namespace string, labels map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	content := map[string]interface{}{
		"apiVersion": "argoproj.io/v1beta1",
		"kind":       "ArgoCD",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]interface{}{},
	}
	if labels != nil {
		content["metadata"].(map[string]interface{})["labels"] = labels
	}
	obj.SetUnstructuredContent(content)
	return obj
}
