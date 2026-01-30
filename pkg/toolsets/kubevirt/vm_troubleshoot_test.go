package kubevirt

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubevirt"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
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
		s.Equal("VirtualMachine Troubleshoot", prompts[0].Prompt.Title)
		s.Len(prompts[0].Prompt.Arguments, 2, "Expected 2 arguments")
	})

	s.Run("returns error for missing namespace", func() {
		prompts := initVMTroubleshoot()
		handler := prompts[0].Handler

		params := api.PromptHandlerParams{
			PromptCallRequest: &mockPromptCallRequest{
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
			PromptCallRequest: &mockPromptCallRequest{
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
}

func TestVMTroubleshoot(t *testing.T) {
	suite.Run(t, new(VMTroubleshootSuite))
}

// Ensure mockPromptCallRequest implements api.PromptCallRequest
var _ api.PromptCallRequest = (*mockPromptCallRequest)(nil)

// Ensure metav1.OwnerReference is available for owner reference tests
var _ metav1.OwnerReference
