package kubernetes

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var podsGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

type CoreTestSuite struct {
	suite.Suite
}

func (s *CoreTestSuite) TestResourcesGet() {
	s.Run("returns existing resource", func() {
		pod := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"name":      "test-pod",
					"namespace": "default",
				},
			},
		}

		fakeClient := newFakeKubernetesClient(
			map[schema.GroupVersionResource]string{podsGVR: "PodList"},
			pod,
		)
		core := NewCore(fakeClient)

		result, err := core.ResourcesGet(ctx(), &schema.GroupVersionKind{
			Group: "", Version: "v1", Kind: "Pod",
		}, "default", "test-pod")

		s.Require().NoError(err)
		s.Equal("test-pod", result.GetName())
		s.Equal("default", result.GetNamespace())
	})

	s.Run("returns error for non-existent resource", func() {
		fakeClient := newFakeKubernetesClient(
			map[schema.GroupVersionResource]string{podsGVR: "PodList"},
		)
		core := NewCore(fakeClient)

		_, err := core.ResourcesGet(ctx(), &schema.GroupVersionKind{
			Group: "", Version: "v1", Kind: "Pod",
		}, "default", "non-existent")

		s.Error(err)
	})
}

func (s *CoreTestSuite) TestResourcesList() {
	s.Run("returns list of resources", func() {
		pod1 := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"name":      "pod-1",
					"namespace": "default",
				},
			},
		}
		pod2 := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"name":      "pod-2",
					"namespace": "default",
				},
			},
		}

		fakeClient := newFakeKubernetesClient(
			map[schema.GroupVersionResource]string{podsGVR: "PodList"},
			pod1, pod2,
		)
		core := NewCore(fakeClient)

		result, err := core.ResourcesList(ctx(), &schema.GroupVersionKind{
			Group: "", Version: "v1", Kind: "Pod",
		}, "default", api.ListOptions{})

		s.Require().NoError(err)
		list, ok := result.(*unstructured.UnstructuredList)
		s.Require().True(ok, "expected *unstructured.UnstructuredList")
		s.Len(list.Items, 2)
	})

	s.Run("returns empty list when no resources exist", func() {
		fakeClient := newFakeKubernetesClient(
			map[schema.GroupVersionResource]string{podsGVR: "PodList"},
		)
		core := NewCore(fakeClient)

		result, err := core.ResourcesList(ctx(), &schema.GroupVersionKind{
			Group: "", Version: "v1", Kind: "Pod",
		}, "default", api.ListOptions{})

		s.Require().NoError(err)
		list, ok := result.(*unstructured.UnstructuredList)
		s.Require().True(ok, "expected *unstructured.UnstructuredList")
		s.Empty(list.Items)
	})
}

func (s *CoreTestSuite) TestResourcesDelete() {
	s.Run("deletes existing resource", func() {
		pod := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"name":      "to-delete",
					"namespace": "default",
				},
			},
		}

		fakeClient := newFakeKubernetesClient(
			map[schema.GroupVersionResource]string{podsGVR: "PodList"},
			pod,
		)
		core := NewCore(fakeClient)

		err := core.ResourcesDelete(ctx(), &schema.GroupVersionKind{
			Group: "", Version: "v1", Kind: "Pod",
		}, "default", "to-delete", nil)

		s.Require().NoError(err)

		// Verify it's gone
		_, err = core.ResourcesGet(ctx(), &schema.GroupVersionKind{
			Group: "", Version: "v1", Kind: "Pod",
		}, "default", "to-delete")
		s.Error(err)
	})
}

func (s *CoreTestSuite) TestResourcesScale() {
	deploymentsGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	s.Run("gets current scale without updating", func() {
		deployment := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name":      "my-deploy",
					"namespace": "default",
				},
				"spec": map[string]interface{}{
					"replicas": int64(3),
				},
			},
		}

		fakeClient := newFakeKubernetesClient(
			map[schema.GroupVersionResource]string{
				podsGVR:        "PodList",
				deploymentsGVR: "DeploymentList",
			},
			deployment,
		)
		core := NewCore(fakeClient)

		result, err := core.ResourcesScale(ctx(), &schema.GroupVersionKind{
			Group: "apps", Version: "v1", Kind: "Deployment",
		}, "default", "my-deploy", 0, false)

		s.Require().NoError(err)
		s.NotNil(result)
	})
}

func (s *CoreTestSuite) TestResourcesCreateOrUpdate() {
	s.Run("creates new resource from YAML", func() {
		resource := `apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  namespace: default
spec:
  containers:
  - name: test-container
    image: nginx:latest`

		fakeClient := newFakeKubernetesClient(
			map[schema.GroupVersionResource]string{podsGVR: "PodList"},
		)
		core := NewCore(fakeClient)

		result, err := core.ResourcesCreateOrUpdate(ctx(), resource)

		s.Require().NoError(err)
		s.NotNil(result)
		s.Require().Len(result, 1)
		s.Equal("test-pod", result[0].GetName())

		// Verify the resource was actually created
		created, err := core.ResourcesGet(ctx(), &schema.GroupVersionKind{
			Group: "", Version: "v1", Kind: "Pod",
		}, "default", "test-pod")
		s.Require().NoError(err)
		s.Equal("test-pod", created.GetName())
	})

	s.Run("updates an existing resource", func() {
		resource := `apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  namespace: default
spec:
  containers:
  - name: test-container
    image: nginx:latest`

		fakeClient := newFakeKubernetesClient(
			map[schema.GroupVersionResource]string{podsGVR: "PodList"},
		)
		core := NewCore(fakeClient)

		result, err := core.ResourcesCreateOrUpdate(ctx(), resource)

		s.Require().NoError(err)
		s.NotNil(result)
		s.Require().Len(result, 1)

		// Verify the resource was actually created
		created, err := core.ResourcesGet(ctx(), &schema.GroupVersionKind{
			Group: "", Version: "v1", Kind: "Pod",
		}, "default", "test-pod")
		s.Require().NoError(err)
		s.Equal("test-pod", created.GetName())
		containers, _, _ := unstructured.NestedSlice(created.Object, "spec", "containers")
		s.Require().Len(containers, 1)
		s.Equal("nginx:latest", containers[0].(map[string]interface{})["image"])

		// Update the resource with a different image
		resource = `apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  namespace: default
spec:
  containers:
  - name: test-container
    image: nginx:1.21`
		result, err = core.ResourcesCreateOrUpdate(ctx(), resource)
		s.Require().NoError(err)
		s.NotNil(result)
		s.Require().Len(result, 1)
		s.Equal("test-pod", result[0].GetName())
		containers, _, _ = unstructured.NestedSlice(result[0].Object, "spec", "containers")
		s.Require().Len(containers, 1)
		s.Equal("nginx:1.21", containers[0].(map[string]interface{})["image"])
	})
}

func TestCore(t *testing.T) {
	suite.Run(t, new(CoreTestSuite))
}

func ctx() context.Context {
	return context.Background()
}
