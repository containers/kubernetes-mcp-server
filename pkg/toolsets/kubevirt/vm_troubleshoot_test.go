package kubevirt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubevirt"
	kubevirtdefaults "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/internal/defaults"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	authv1client "k8s.io/client-go/kubernetes/typed/authorization/v1"
	"k8s.io/client-go/rest"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/clientcmd"
	metricsv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned/typed/metrics/v1beta1"
)

// mockPromptCallRequest implements api.PromptCallRequest for testing
type mockPromptCallRequest struct {
	args map[string]string
}

func (m *mockPromptCallRequest) GetArguments() map[string]string {
	return m.args
}

type VMTroubleshootSuite struct {
	suite.Suite
}

func (s *VMTroubleshootSuite) TestVMTroubleshootPrompt() {
	s.Run("prompt is registered", func() {
		prompts := initVMTroubleshoot()
		s.Require().Len(prompts, 1, "Expected 1 prompt")
		s.Equal("vm-troubleshoot", prompts[0].Prompt.Name)
		s.Equal(fmt.Sprintf("%s VirtualMachine Troubleshoot", kubevirtdefaults.ProductName()), prompts[0].Prompt.Title)
		s.Len(prompts[0].Prompt.Arguments, 2, "Expected 2 arguments")
	})

	s.Run("returns error for missing namespace", func() {
		prompts := initVMTroubleshoot()
		handler := prompts[0].Handler

		params := api.PromptHandlerParams{
			Request: &mockPromptCallRequest{
				args: map[string]string{
					"name": "test-vm",
				},
			},
		}

		result, err := handler(params)
		s.Error(err)
		s.Nil(result)
		s.Contains(err.Error(), "namespace")
	})

	s.Run("returns error for missing name", func() {
		prompts := initVMTroubleshoot()
		handler := prompts[0].Handler

		params := api.PromptHandlerParams{
			Request: &mockPromptCallRequest{
				args: map[string]string{
					"namespace": "test-ns",
				},
			},
		}

		result, err := handler(params)
		s.Error(err)
		s.Nil(result)
		s.Contains(err.Error(), "name")
	})
}

func (s *VMTroubleshootSuite) TestFetchVirtualMachineStatus() {
	ctx := context.Background()
	gvrToListKind := map[schema.GroupVersionResource]string{
		kubevirt.VirtualMachineGVR: "VirtualMachineList",
	}

	s.Run("returns VM status yaml when found", func() {
		testVM := &unstructured.Unstructured{}
		testVM.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata": map[string]interface{}{
				"name":      "test-vm",
				"namespace": "test-ns",
			},
			"spec": map[string]interface{}{
				"running": true,
			},
			"status": map[string]interface{}{
				"printableStatus": "Running",
				"ready":           true,
			},
		})

		client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind, testVM)
		result, vm := fetchVirtualMachineStatus(ctx, client, "test-ns", "test-vm")

		s.Contains(result, "### VirtualMachine Status: test-ns/test-vm")
		s.Contains(result, "printableStatus")
		s.Contains(result, "Running")
		s.NotNil(vm)
	})

	s.Run("returns error message when VM not found", func() {
		client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind)
		result, vm := fetchVirtualMachineStatus(ctx, client, "test-ns", "nonexistent-vm")

		s.Contains(result, "*Error fetching VirtualMachine:")
		s.Contains(result, "not found*")
		s.Nil(vm)
	})
}

func (s *VMTroubleshootSuite) TestFetchVirtualMachineInstanceStatus() {
	ctx := context.Background()
	gvrToListKind := map[schema.GroupVersionResource]string{
		kubevirt.VirtualMachineInstanceGVR: "VirtualMachineInstanceList",
	}

	s.Run("returns VMI status yaml when found", func() {
		testVMI := &unstructured.Unstructured{}
		testVMI.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachineInstance",
			"metadata": map[string]interface{}{
				"name":      "test-vm",
				"namespace": "test-ns",
			},
			"status": map[string]interface{}{
				"phase": "Running",
			},
		})

		client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind, testVMI)
		result, vmi := fetchVirtualMachineInstanceStatus(ctx, client, "test-ns", "test-vm")

		s.Contains(result, "### VirtualMachineInstance Status: test-ns/test-vm")
		s.Contains(result, "phase")
		s.Contains(result, "Running")
		s.NotNil(vmi)
	})

	s.Run("returns info message when VMI not found", func() {
		client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind)
		result, vmi := fetchVirtualMachineInstanceStatus(ctx, client, "test-ns", "nonexistent-vm")

		s.Contains(result, "*VirtualMachineInstance not found or error:")
		s.Contains(result, "not found*")
		s.Nil(vmi)
	})
}

func (s *VMTroubleshootSuite) TestFetchVirtualMachineVolumes() {
	s.Run("returns volumes list from VM when found", func() {
		testVM := &unstructured.Unstructured{}
		testVM.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata": map[string]interface{}{
				"name":      "test-vm",
				"namespace": "test-ns",
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"volumes": []interface{}{
							map[string]interface{}{
								"name": "rootdisk",
								"containerDisk": map[string]interface{}{
									"image": "quay.io/containerdisks/fedora:latest",
								},
							},
							map[string]interface{}{
								"name": "cloudinit",
								"cloudInitNoCloud": map[string]interface{}{
									"userData": "#cloud-config",
								},
							},
						},
					},
				},
			},
		})

		result := fetchVirtualMachineVolumes("test-ns", "test-vm", testVM, nil)

		s.Contains(result, "### Configured Volumes (from VirtualMachine: test-ns/test-vm)")
		s.Contains(result, "rootdisk")
		s.Contains(result, "cloudinit")
	})

	s.Run("returns volumes list from VMI when VM is nil", func() {
		testVMI := &unstructured.Unstructured{}
		testVMI.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachineInstance",
			"metadata": map[string]interface{}{
				"name":      "test-vm",
				"namespace": "test-ns",
			},
			"spec": map[string]interface{}{
				"volumes": []interface{}{
					map[string]interface{}{
						"name": "rootdisk",
						"containerDisk": map[string]interface{}{
							"image": "quay.io/containerdisks/fedora:latest",
						},
					},
				},
			},
		})

		result := fetchVirtualMachineVolumes("test-ns", "test-vm", nil, testVMI)

		s.Contains(result, "### Configured Volumes (from VirtualMachineInstance: test-ns/test-vm)")
		s.Contains(result, "rootdisk")
	})

	s.Run("returns volumes from VMI when VM has no volumes", func() {
		testVM := &unstructured.Unstructured{}
		testVM.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata": map[string]interface{}{
				"name":      "test-vm",
				"namespace": "test-ns",
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{},
				},
			},
		})

		testVMI := &unstructured.Unstructured{}
		testVMI.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachineInstance",
			"metadata": map[string]interface{}{
				"name":      "test-vm",
				"namespace": "test-ns",
			},
			"spec": map[string]interface{}{
				"volumes": []interface{}{
					map[string]interface{}{
						"name": "rootdisk",
						"containerDisk": map[string]interface{}{
							"image": "quay.io/containerdisks/fedora:latest",
						},
					},
				},
			},
		})

		result := fetchVirtualMachineVolumes("test-ns", "test-vm", testVM, testVMI)

		s.Contains(result, "### Configured Volumes (from VirtualMachineInstance: test-ns/test-vm)")
		s.Contains(result, "rootdisk")
	})

	s.Run("returns message when no volumes found in VM or VMI", func() {
		testVM := &unstructured.Unstructured{}
		testVM.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata": map[string]interface{}{
				"name":      "test-vm",
				"namespace": "test-ns",
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{},
				},
			},
		})

		result := fetchVirtualMachineVolumes("test-ns", "test-vm", testVM, nil)

		s.Contains(result, "*No volumes configured*")
	})

	s.Run("returns message when both VM and VMI are nil", func() {
		result := fetchVirtualMachineVolumes("test-ns", "test-vm", nil, nil)

		s.Contains(result, "*No volumes configured*")
	})
}

func (s *VMTroubleshootSuite) TestFetchVirtLauncherPod() {
	ctx := context.Background()
	gvrToListKind := map[schema.GroupVersionResource]string{
		kubevirt.PodGVR: "PodList",
	}

	s.Run("returns virt-launcher pod and pod names", func() {
		testPod := &unstructured.Unstructured{}
		testPod.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "virt-launcher-test-vm-abc123",
				"namespace": "test-ns",
				"labels": map[string]interface{}{
					"kubevirt.io":         "virt-launcher",
					"vm.kubevirt.io/name": "test-vm",
				},
			},
			"status": map[string]interface{}{
				"phase": "Running",
			},
		})

		client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind, testPod)
		result, podNames := fetchVirtLauncherPod(ctx, client, "test-ns", "test-vm")

		s.Contains(result, "### virt-launcher Pod")
		s.Contains(result, "#### virt-launcher-test-vm-abc123")
		s.Contains(result, "kind: Pod")
		s.Len(podNames, 1)
		s.Equal("virt-launcher-test-vm-abc123", podNames[0])
	})

	s.Run("returns message when no pod found", func() {
		client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind)
		result, podNames := fetchVirtLauncherPod(ctx, client, "test-ns", "test-vm")

		s.Contains(result, "*No virt-launcher pod found*")
		s.Contains(result, "This is expected if the VM is stopped")
		s.Nil(podNames)
	})

	s.Run("excludes pods for other VMs", func() {
		otherPod := &unstructured.Unstructured{}
		otherPod.SetUnstructuredContent(map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "virt-launcher-other-vm-xyz789",
				"namespace": "test-ns",
				"labels": map[string]interface{}{
					"kubevirt.io":         "virt-launcher",
					"vm.kubevirt.io/name": "other-vm",
				},
			},
		})

		client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind, otherPod)
		result, podNames := fetchVirtLauncherPod(ctx, client, "test-ns", "test-vm")

		s.Contains(result, "*No virt-launcher pod found*")
		s.Nil(podNames)
	})
}

func (s *VMTroubleshootSuite) TestFetchVirtLauncherPodLogs() {
	s.Run("returns message when no pod names provided", func() {
		result := fetchVirtLauncherPodLogs(context.Background(), nil, "test-ns", nil)

		s.Contains(result, "### virt-launcher Pod Logs")
		s.Contains(result, "*No virt-launcher pod found - no logs available*")
	})

	s.Run("returns message when empty pod names provided", func() {
		result := fetchVirtLauncherPodLogs(context.Background(), nil, "test-ns", []string{})

		s.Contains(result, "*No virt-launcher pod found - no logs available*")
	})

	s.Run("keeps earlier logs when a later pod fails", func() {
		var pods []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pods = append(pods, r.URL.Path)
			if strings.Contains(r.URL.Path, "/pods/pod-b/") {
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
			_, _ = io.WriteString(w, "log-from-"+r.URL.Path)
		}))
		s.T().Cleanup(srv.Close)
		cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
		s.Require().NoError(err)

		result := fetchVirtLauncherPodLogs(context.Background(), promptKubeClient{Interface: cs}, "test-ns", []string{"pod-a", "pod-b", "pod-c"})

		s.Contains(result, "log-from-")
		s.Contains(result, "pod-a")
		s.Contains(result, "*Error fetching logs:")
		s.Contains(result, "boom")
		s.Contains(result, "pod-c")
		s.GreaterOrEqual(len(pods), 3)
	})
}

func (s *VMTroubleshootSuite) TestFetchEvents() {
	eventGVR := schema.GroupVersionResource{Version: "v1", Resource: "events"}
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Version: "v1"}})
	mapper.AddSpecific(
		schema.GroupVersionKind{Version: "v1", Kind: "Event"},
		eventGVR,
		schema.GroupVersionResource{Version: "v1", Resource: "event"},
		meta.RESTScopeNamespace,
	)
	newClient := func(reactor clienttesting.ReactionFunc) promptKubeClient {
		dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
			eventGVR: "EventList",
		})
		dyn.PrependReactor("list", "events", reactor)
		return promptKubeClient{dyn: dyn, mapper: promptResettingMapper{RESTMapper: mapper}}
	}
	vmEvent := func(name, message string) *unstructured.UnstructuredList {
		return &unstructured.UnstructuredList{Items: []unstructured.Unstructured{{Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Event",
			"metadata":   map[string]any{"name": name, "namespace": "test-ns"},
			"message":    message,
			"reason":     "Failed",
			"type":       "Warning",
			"involvedObject": map[string]any{
				"apiVersion": "kubevirt.io/v1",
				"kind":       "VirtualMachine",
				"name":       "test-vm",
			},
		}}}}
	}

	s.Run("keeps VM events when the namespace list fails", func() {
		calls := 0
		result := fetchEvents(context.Background(), newClient(func(clienttesting.Action) (bool, runtime.Object, error) {
			calls++
			if calls == 1 {
				return true, vmEvent("vm-event", "disk missing"), nil
			}
			return true, nil, errors.New("namespace list failed")
		}), "test-ns", "test-vm")

		s.Contains(result, "disk missing")
		s.Contains(result, "*Error listing events:")
		s.Contains(result, "namespace list failed")
		s.Equal(2, calls)
	})

	s.Run("keeps launcher events when the VM list fails", func() {
		calls := 0
		result := fetchEvents(context.Background(), newClient(func(clienttesting.Action) (bool, runtime.Object, error) {
			calls++
			if calls == 1 {
				return true, nil, errors.New("selector failed")
			}
			list := vmEvent("pod-event", "launcher died")
			list.Items[0].Object["involvedObject"] = map[string]any{
				"apiVersion": "v1",
				"kind":       "Pod",
				"name":       "virt-launcher-test-vm-abc",
			}
			return true, list, nil
		}), "test-ns", "test-vm")

		s.Contains(result, "launcher died")
		s.Contains(result, "*Error listing events:")
		s.Contains(result, "selector failed")
		s.Equal(2, calls)
	})
}

// promptKubeClient is a Kubernetes client for prompt retention tests.
type promptKubeClient struct {
	kubernetes.Interface
	dyn    dynamic.Interface
	mapper meta.ResettableRESTMapper
}

func (promptKubeClient) NamespaceOrDefault(ns string) string { return ns }
func (promptKubeClient) ToRESTConfig() (*rest.Config, error) {
	return nil, errPromptClientUnused
}
func (promptKubeClient) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	return nil, errPromptClientUnused
}
func (promptKubeClient) ToRESTMapper() (meta.RESTMapper, error) { return nil, errPromptClientUnused }
func (promptKubeClient) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return nil
}
func (promptKubeClient) RESTConfig() *rest.Config { return nil }
func (c promptKubeClient) RESTMapper() meta.ResettableRESTMapper {
	return c.mapper
}
func (promptKubeClient) DiscoveryClient() discovery.CachedDiscoveryInterface {
	return promptEventDiscovery{}
}
func (c promptKubeClient) DynamicClient() dynamic.Interface { return c.dyn }
func (promptKubeClient) AuthorizationV1() authv1client.AuthorizationV1Interface {
	return nil
}
func (promptKubeClient) MetricsV1beta1Client() *metricsv1beta1.MetricsV1beta1Client {
	return nil
}

type promptResettingMapper struct{ meta.RESTMapper }

func (promptResettingMapper) Reset() {}

type promptEventDiscovery struct{ discovery.DiscoveryInterface }

func (promptEventDiscovery) ServerResourcesForGroupVersion(string) (*metav1.APIResourceList, error) {
	return &metav1.APIResourceList{
		GroupVersion: "v1",
		APIResources: []metav1.APIResource{{Name: "events", Kind: "Event", Namespaced: true}},
	}, nil
}
func (promptEventDiscovery) Fresh() bool { return true }
func (promptEventDiscovery) Invalidate() {}

var errPromptClientUnused = errors.New("unused")

func TestVMTroubleshoot(t *testing.T) {
	suite.Run(t, new(VMTroubleshootSuite))
}

// Ensure mockPromptCallRequest implements api.PromptCallRequest
var _ api.PromptCallRequest = (*mockPromptCallRequest)(nil)

// Ensure metav1.OwnerReference is available for owner reference tests
var _ metav1.OwnerReference
