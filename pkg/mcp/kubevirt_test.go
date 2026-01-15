package mcp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/internal/test"
	kubevirttesting "github.com/containers/kubernetes-mcp-server/pkg/kubevirt/testing"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"
)

var kubevirtApis = []schema.GroupVersionResource{
	{Group: "kubevirt.io", Version: "v1", Resource: "virtualmachines"},
	{Group: "cdi.kubevirt.io", Version: "v1beta1", Resource: "datasources"},
	{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachineclusterinstancetypes"},
	{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachineinstancetypes"},
	{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachineclusterpreferences"},
	{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachinepreferences"},
}

type KubevirtSuite struct {
	BaseMcpSuite
}

func (s *KubevirtSuite) SetupSuite() {
	ctx := s.T().Context()
	tasks, _ := errgroup.WithContext(ctx)
	for _, api := range kubevirtApis {
		gvr := api // capture loop variable
		tasks.Go(func() error { return EnvTestEnableCRD(ctx, gvr.Group, gvr.Version, gvr.Resource) })
	}
	s.Require().NoError(tasks.Wait())

	_, err := kubernetes.NewForConfigOrDie(envTestRestConfig).CoreV1().Namespaces().
		Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "openshift-virtualization-os-images"}}, metav1.CreateOptions{})
	s.Require().NoError(err, "failed to create test namespace openshift-virtualization-os-images")
}

func (s *KubevirtSuite) TearDownSuite() {
	tasks, _ := errgroup.WithContext(s.T().Context())
	for _, api := range kubevirtApis {
		gvr := api // capture loop variable
		tasks.Go(func() error { return EnvTestDisableCRD(s.T().Context(), gvr.Group, gvr.Version, gvr.Resource) })
	}
	s.Require().NoError(tasks.Wait())
}

func (s *KubevirtSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.Require().NoError(toml.Unmarshal([]byte(`
		toolsets = [ "kubevirt" ]
	`), s.Cfg), "Expected to parse toolsets config")
	s.InitMcpClient()
}

func (s *KubevirtSuite) TestCreate() {
	s.Run("vm_create missing required params", func() {
		testCases := []string{"name", "namespace"}
		for _, param := range testCases {
			s.Run("missing "+param, func() {
				params := map[string]interface{}{
					"name":      "test-vm",
					"namespace": "default",
				}
				delete(params, param)
				toolResult, err := s.CallTool("vm_create", params)
				s.Require().Nilf(err, "call tool failed %v", err)
				s.Truef(toolResult.IsError, "expected call tool to fail due to missing %s", param)
				s.Equal(toolResult.Content[0].(mcp.TextContent).Text, param+" parameter required")
			})
		}
	})
	s.Run("vm_create with default settings", func() {
		toolResult, err := s.CallTool("vm_create", map[string]interface{}{
			"name":      "test-vm",
			"namespace": "default",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		var decodedResult []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decodedResult)
		s.Run("returns yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
			s.Truef(strings.HasPrefix(toolResult.Content[0].(mcp.TextContent).Text, "# VirtualMachine created successfully"),
				"Expected success message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
			s.Require().Lenf(decodedResult, 1, "invalid resource count, expected 1, got %v", len(decodedResult))
			vm := &decodedResult[0]
			s.Equal("test-vm", vm.GetName(), "invalid resource name")
			s.Equal("default", vm.GetNamespace(), "invalid resource namespace")
			s.NotEmptyf(vm.GetUID(), "invalid uid, got %v", vm.GetUID())
			s.Equal("quay.io/containerdisks/fedora:latest", test.FieldString(vm, "spec.template.spec.volumes[0].containerDisk.image"), "invalid default image")
			s.Equal("Halted", test.FieldString(vm, "spec.runStrategy"), "invalid default runStrategy")
		})
	})
	s.Run("vm_create(workload=ubuntu, instancetype=u1.medium) with instancetype", func() {
		toolResult, err := s.CallTool("vm_create", map[string]interface{}{
			"name":         "test-vm-2",
			"namespace":    "default",
			"workload":     "ubuntu",
			"instancetype": "u1.medium",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		var decodedResult []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decodedResult)
		s.Run("returns yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
			s.Truef(strings.HasPrefix(toolResult.Content[0].(mcp.TextContent).Text, "# VirtualMachine created successfully"),
				"Expected success message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
			s.Require().Lenf(decodedResult, 1, "invalid resource count, expected 1, got %v", len(decodedResult))
			vm := &decodedResult[0]
			s.Equal("test-vm-2", vm.GetName(), "invalid resource name")
			s.Equal("default", vm.GetNamespace(), "invalid resource namespace")
			s.NotEmptyf(vm.GetUID(), "invalid uid, got %v", vm.GetUID())
			s.Equal("quay.io/containerdisks/ubuntu:24.04", test.FieldString(vm, "spec.template.spec.volumes[0].containerDisk.image"), "invalid image for ubuntu workload")
			s.Equal("Halted", test.FieldString(vm, "spec.runStrategy"), "invalid default runStrategy")
			s.Equal("VirtualMachineClusterInstancetype", test.FieldString(vm, "spec.instancetype.kind"), "invalid memory for u1.medium instanceType")
			s.Equal("u1.medium", test.FieldString(vm, "spec.instancetype.name"), "invalid cpu cores for u1.medium instanceType")
		})
	})
	s.Run("vm_create(workload=rhel, preference=rhel.9) with preference", func() {
		toolResult, err := s.CallTool("vm_create", map[string]interface{}{
			"name":       "test-vm-3",
			"namespace":  "default",
			"workload":   "rhel",
			"preference": "rhel.9",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		var decodedResult []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decodedResult)
		s.Run("returns yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
			s.Truef(strings.HasPrefix(toolResult.Content[0].(mcp.TextContent).Text, "# VirtualMachine created successfully"),
				"Expected success message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
			s.Require().Lenf(decodedResult, 1, "invalid resource count, expected 1, got %v", len(decodedResult))
			vm := &decodedResult[0]
			s.Equal("test-vm-3", vm.GetName(), "invalid resource name")
			s.Equal("default", vm.GetNamespace(), "invalid resource namespace")
			s.NotEmptyf(vm.GetUID(), "invalid uid, got %v", vm.GetUID())
			s.Equal("rhel", test.FieldString(vm, "spec.template.spec.volumes[0].containerDisk.image"), "invalid image for rhel workload")
			s.Equal("Halted", test.FieldString(vm, "spec.runStrategy"), "invalid default runStrategy")
			s.Equal("VirtualMachineClusterPreference", test.FieldString(vm, "spec.preference.kind"), "invalid preference kind for rhel.9 preference")
			s.Equal("rhel.9", test.FieldString(vm, "spec.preference.name"), "invalid preference name for rhel.9 preference")
		})
	})
	s.Run("vm_create(workload=quay.io/myrepo/myimage:v1.0) with custom container disk", func() {
		toolResult, err := s.CallTool("vm_create", map[string]interface{}{
			"name":      "test-vm-4",
			"namespace": "default",
			"workload":  "quay.io/myrepo/myimage:v1.0",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		var decodedResult []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decodedResult)
		s.Run("returns yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
			s.Truef(strings.HasPrefix(toolResult.Content[0].(mcp.TextContent).Text, "# VirtualMachine created successfully"),
				"Expected success message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
			s.Require().Lenf(decodedResult, 1, "invalid resource count, expected 1, got %v", len(decodedResult))
			vm := &decodedResult[0]
			s.Equal("test-vm-4", vm.GetName(), "invalid resource name")
			s.Equal("default", vm.GetNamespace(), "invalid resource namespace")
			s.NotEmptyf(vm.GetUID(), "invalid uid, got %v", vm.GetUID())
			s.Equal("quay.io/myrepo/myimage:v1.0", test.FieldString(vm, "spec.template.spec.volumes[0].containerDisk.image"), "invalid image for custom container disk workload")
			s.Equal("Halted", test.FieldString(vm, "spec.runStrategy"), "invalid default runStrategy")
		})
	})
	s.Run("with size", func() {
		dynamicClient := dynamic.NewForConfigOrDie(envTestRestConfig).Resource(
			schema.GroupVersionResource{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachineclusterinstancetypes"},
		)
		instanceTypes := []struct{ instanceType, performance string }{
			{"compute", "c1"},
			{"general", "u1"},
			{"memory", "m1"},
		}
		for _, size := range []string{"medium", "small", "large"} {
			for _, instanceType := range instanceTypes {
				labels := map[string]string{}
				labels["instancetype.kubevirt.io/class"] = instanceType.instanceType
				_, err := dynamicClient.Create(
					s.T().Context(),
					kubevirttesting.NewUnstructuredInstancetype(fmt.Sprintf("%s.%s", instanceType.performance, size), labels),
					metav1.CreateOptions{},
				)
				s.Require().NoError(err)
			}
		}

		s.Run("vm_create(size=medium) with size hint matching instancetype", func() {
			toolResult, err := s.CallTool("vm_create", map[string]interface{}{
				"name":      "test-vm-5",
				"namespace": "default",
				"size":      "medium",
			})
			s.Run("no error", func() {
				s.Nilf(err, "call tool failed %v", err)
				s.Falsef(toolResult.IsError, "call tool failed")
			})
			var decodedResult []unstructured.Unstructured
			err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decodedResult)
			s.Run("returns yaml content", func() {
				s.Nilf(err, "invalid tool result content %v", err)
				s.Truef(strings.HasPrefix(toolResult.Content[0].(mcp.TextContent).Text, "# VirtualMachine created successfully"),
					"Expected success message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
				s.Require().Lenf(decodedResult, 1, "invalid resource count, expected 1, got %v", len(decodedResult))
				vm := &decodedResult[0]
				s.Equal("test-vm-5", vm.GetName(), "invalid resource name")
				s.Equal("default", vm.GetNamespace(), "invalid resource namespace")
				s.NotEmptyf(vm.GetUID(), "invalid uid, got %v", vm.GetUID())
				s.Equal("VirtualMachineClusterInstancetype", test.FieldString(vm, "spec.instancetype.kind"), "invalid instanceType kind for medium size hint")
				s.Equal("u1.medium", test.FieldString(vm, "spec.instancetype.name"), "invalid instanceType name for medium size hint")
			})
		})
		s.Run("vm_create(size=large, performance=compute-optimized) with size and performance hints", func() {
			toolResult, err := s.CallTool("vm_create", map[string]interface{}{
				"name":        "test-vm-6",
				"namespace":   "default",
				"size":        "large",
				"performance": "compute-optimized",
			})
			s.Run("no error", func() {
				s.Nilf(err, "call tool failed %v", err)
				s.Falsef(toolResult.IsError, "call tool failed")
			})
			var decodedResult []unstructured.Unstructured
			err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decodedResult)
			s.Run("returns yaml content", func() {
				s.Nilf(err, "invalid tool result content %v", err)
				s.Truef(strings.HasPrefix(toolResult.Content[0].(mcp.TextContent).Text, "# VirtualMachine created successfully"),
					"Expected success message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
				s.Require().Lenf(decodedResult, 1, "invalid resource count, expected 1, got %v", len(decodedResult))
				vm := &decodedResult[0]
				s.Equal("test-vm-6", vm.GetName(), "invalid resource name")
				s.Equal("default", vm.GetNamespace(), "invalid resource namespace")
				s.NotEmptyf(vm.GetUID(), "invalid uid, got %v", vm.GetUID())
				s.Equal("VirtualMachineClusterInstancetype", test.FieldString(vm, "spec.instancetype.kind"), "invalid instanceType kind for large size hint")
				s.Equal("c1.large", test.FieldString(vm, "spec.instancetype.name"), "invalid instanceType name for large size hint")
			})
		})
		s.Run("vm_create(size=xlarge) with size hint not matching any instancetype", func() {
			toolResult, err := s.CallTool("vm_create", map[string]interface{}{
				"name":      "test-vm-7",
				"namespace": "default",
				"size":      "xlarge",
			})
			s.Run("no error", func() {
				s.Nilf(err, "call tool failed %v", err)
				s.Falsef(toolResult.IsError, "call tool failed")
			})
			var decodedResult []unstructured.Unstructured
			err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decodedResult)
			s.Run("returns yaml content", func() {
				s.Nilf(err, "invalid tool result content %v", err)
				s.Truef(strings.HasPrefix(toolResult.Content[0].(mcp.TextContent).Text, "# VirtualMachine created successfully"),
					"Expected success message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
				s.Require().Lenf(decodedResult, 1, "invalid resource count, expected 1, got %v", len(decodedResult))
				vm := &decodedResult[0]
				s.Equal("test-vm-7", vm.GetName(), "invalid resource name")
				s.Equal("default", vm.GetNamespace(), "invalid resource namespace")
				s.NotEmptyf(vm.GetUID(), "invalid uid, got %v", vm.GetUID())
				s.Falsef(test.FieldExists(vm, "spec.instancetype"), "expected no instancetype to be set for xlarge size hint")
			})
		})
	})
	s.Run("with data sources", func() {
		dynamicClient := dynamic.NewForConfigOrDie(envTestRestConfig).Resource(
			schema.GroupVersionResource{Group: "cdi.kubevirt.io", Version: "v1beta1", Resource: "datasources"},
		)
		_, err := dynamicClient.Namespace("openshift-virtualization-os-images").Create(
			s.T().Context(),
			kubevirttesting.NewUnstructuredDataSource("fedora", "openshift-virtualization-os-images", "registry.redhat.io/fedora:latest", "u1.medium", "fedora"),
			metav1.CreateOptions{},
		)
		s.Require().NoError(err)

		s.Run("vm_create(workload=fedora) using DataSource with default instancetype and preference", func() {
			toolResult, err := s.CallTool("vm_create", map[string]interface{}{
				"name":      "test-vm-8",
				"namespace": "default",
				"workload":  "fedora",
			})
			s.Run("no error", func() {
				s.Nilf(err, "call tool failed %v", err)
				s.Falsef(toolResult.IsError, "call tool failed")
			})
			var decodedResult []unstructured.Unstructured
			err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decodedResult)
			s.Run("returns yaml content", func() {
				s.Nilf(err, "invalid tool result content %v", err)
				s.Truef(strings.HasPrefix(toolResult.Content[0].(mcp.TextContent).Text, "# VirtualMachine created successfully"),
					"Expected success message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
				s.Require().Lenf(decodedResult, 1, "invalid resource count, expected 1, got %v", len(decodedResult))
				vm := &decodedResult[0]
				s.Equal("test-vm-8", vm.GetName(), "invalid resource name")
				s.Equal("default", vm.GetNamespace(), "invalid resource namespace")
				s.NotEmptyf(vm.GetUID(), "invalid uid, got %v", vm.GetUID())
				s.Equal("Halted", test.FieldString(vm, "spec.runStrategy"), "invalid default runStrategy")
				s.Equal("VirtualMachineClusterInstancetype", test.FieldString(vm, "spec.instancetype.kind"), "invalid instanceType kind from DataSource default")
				s.Equal("u1.medium", test.FieldString(vm, "spec.instancetype.name"), "invalid instanceType name from DataSource default")
				s.Equal("VirtualMachineClusterPreference", test.FieldString(vm, "spec.preference.kind"), "invalid preference kind from DataSource default")
				s.Equal("fedora", test.FieldString(vm, "spec.preference.name"), "invalid preference name from DataSource default")
				s.Equal("DataSource", test.FieldString(vm, "spec.dataVolumeTemplates[0].spec.sourceRef.kind"), "invalid data source kind in dataVolumeTemplates")
				s.Equal("fedora", test.FieldString(vm, "spec.dataVolumeTemplates[0].spec.sourceRef.name"), "invalid data source name in dataVolumeTemplates")
			})
		})
		s.Run("vm_create(workload=rhel) using DataSource partial name match", func() {
			_, err := dynamicClient.Namespace("openshift-virtualization-os-images").Create(
				s.T().Context(),
				kubevirttesting.NewUnstructuredDataSource("rhel9", "openshift-virtualization-os-images", "registry.redhat.io/rhel9:latest", "", "rhel.9"),
				metav1.CreateOptions{},
			)
			s.Require().NoError(err)

			toolResult, err := s.CallTool("vm_create", map[string]interface{}{
				"name":      "test-vm-9",
				"namespace": "default",
				"workload":  "rhel",
			})
			s.Run("no error", func() {
				s.Nilf(err, "call tool failed %v", err)
				s.Falsef(toolResult.IsError, "call tool failed")
			})
			var decodedResult []unstructured.Unstructured
			err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decodedResult)
			s.Run("returns yaml content", func() {
				s.Nilf(err, "invalid tool result content %v", err)
				s.Truef(strings.HasPrefix(toolResult.Content[0].(mcp.TextContent).Text, "# VirtualMachine created successfully"),
					"Expected success message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
				s.Require().Lenf(decodedResult, 1, "invalid resource count, expected 1, got %v", len(decodedResult))
				vm := &decodedResult[0]
				s.Equal("test-vm-9", vm.GetName(), "invalid resource name")
				s.Equal("default", vm.GetNamespace(), "invalid resource namespace")
				s.NotEmptyf(vm.GetUID(), "invalid uid, got %v", vm.GetUID())
				s.Equal("Halted", test.FieldString(vm, "spec.runStrategy"), "invalid default runStrategy")
				s.Equal("VirtualMachineClusterPreference", test.FieldString(vm, "spec.preference.kind"), "invalid preference kind from DataSource default")
				s.Equal("rhel.9", test.FieldString(vm, "spec.preference.name"), "invalid preference name from DataSource default")
				s.Equal("DataSource", test.FieldString(vm, "spec.dataVolumeTemplates[0].spec.sourceRef.kind"), "invalid data source kind in dataVolumeTemplates")
				s.Equal("rhel9", test.FieldString(vm, "spec.dataVolumeTemplates[0].spec.sourceRef.name"), "invalid data source name in dataVolumeTemplates")
			})
		})
		s.Run("vm_create(workload=fedora, size=large) with size hint overriding DataSource default instancetype", func() {
			toolResult, err := s.CallTool("vm_create", map[string]interface{}{
				"name":      "test-vm-10",
				"namespace": "default",
				"workload":  "fedora",
				"size":      "large",
			})
			s.Run("no error", func() {
				s.Nilf(err, "call tool failed %v", err)
				s.Falsef(toolResult.IsError, "call tool failed")
			})
			var decodedResult []unstructured.Unstructured
			err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decodedResult)
			s.Run("returns yaml content", func() {
				s.Nilf(err, "invalid tool result content %v", err)
				s.Truef(strings.HasPrefix(toolResult.Content[0].(mcp.TextContent).Text, "# VirtualMachine created successfully"),
					"Expected success message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
				s.Require().Lenf(decodedResult, 1, "invalid resource count, expected 1, got %v", len(decodedResult))
				vm := &decodedResult[0]
				s.Equal("test-vm-10", vm.GetName(), "invalid resource name")
				s.Equal("default", vm.GetNamespace(), "invalid resource namespace")
				s.NotEmptyf(vm.GetUID(), "invalid uid, got %v", vm.GetUID())
				s.Equal("Halted", test.FieldString(vm, "spec.runStrategy"), "invalid default runStrategy")
				s.Equal("VirtualMachineClusterInstancetype", test.FieldString(vm, "spec.instancetype.kind"), "invalid instanceType kind for large size hint")
				s.Equal("u1.large", test.FieldString(vm, "spec.instancetype.name"), "invalid instanceType name for large size hint")
			})
		})
	})
	s.Run("with preferences", func() {
		dynamicClient := dynamic.NewForConfigOrDie(envTestRestConfig).Resource(
			schema.GroupVersionResource{Group: "instancetype.kubevirt.io", Version: "v1beta1", Resource: "virtualmachineclusterpreferences"},
		)
		for _, preference := range []*unstructured.Unstructured{
			kubevirttesting.NewUnstructuredPreference("rhel.9", false),
			kubevirttesting.NewUnstructuredPreference("fedora", false),
		} {
			_, err := dynamicClient.Create(s.T().Context(), preference, metav1.CreateOptions{})
			s.Require().NoError(err)
		}

		s.Run("vm_create(workload=rhel) auto-select preference matching workload name", func() {
			toolResult, err := s.CallTool("vm_create", map[string]interface{}{
				"name":      "test-vm-11",
				"namespace": "default",
				"workload":  "rhel",
			})
			s.Run("no error", func() {
				s.Nilf(err, "call tool failed %v", err)
				s.Falsef(toolResult.IsError, "call tool failed")
			})
			var decodedResult []unstructured.Unstructured
			err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decodedResult)
			s.Run("returns yaml content", func() {
				s.Nilf(err, "invalid tool result content %v", err)
				s.Truef(strings.HasPrefix(toolResult.Content[0].(mcp.TextContent).Text, "# VirtualMachine created successfully"),
					"Expected success message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
				s.Require().Lenf(decodedResult, 1, "invalid resource count, expected 1, got %v", len(decodedResult))
				vm := &decodedResult[0]
				s.Equal("test-vm-11", vm.GetName(), "invalid resource name")
				s.Equal("default", vm.GetNamespace(), "invalid resource namespace")
				s.NotEmptyf(vm.GetUID(), "invalid uid, got %v", vm.GetUID())
				s.Equal("VirtualMachineClusterPreference", test.FieldString(vm, "spec.preference.kind"), "invalid preference kind for rhel.9 preference")
				s.Equal("rhel.9", test.FieldString(vm, "spec.preference.name"), "invalid preference name for rhel.9 preference")
			})
		})
		s.Run("vm_create(workload=fedora, preference=custom.preference) with explicit preference overriding auto-selected preference", func() {
			toolResult, err := s.CallTool("vm_create", map[string]interface{}{
				"name":       "test-vm-12",
				"namespace":  "default",
				"workload":   "fedora",
				"preference": "custom.preference",
			})
			s.Run("no error", func() {
				s.Nilf(err, "call tool failed %v", err)
				s.Falsef(toolResult.IsError, "call tool failed")
			})
			var decodedResult []unstructured.Unstructured
			err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decodedResult)
			s.Run("returns yaml content", func() {
				s.Nilf(err, "invalid tool result content %v", err)
				s.Truef(strings.HasPrefix(toolResult.Content[0].(mcp.TextContent).Text, "# VirtualMachine created successfully"),
					"Expected success message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
				s.Require().Lenf(decodedResult, 1, "invalid resource count, expected 1, got %v", len(decodedResult))
				vm := &decodedResult[0]
				s.Equal("test-vm-12", vm.GetName(), "invalid resource name")
				s.Equal("default", vm.GetNamespace(), "invalid resource namespace")
				s.NotEmptyf(vm.GetUID(), "invalid uid, got %v", vm.GetUID())
				s.Equal("custom.preference", test.FieldString(vm, "spec.preference.name"), "invalid preference name for explicit custom.preference")
			})
		})
	})
}

func (s *KubevirtSuite) TestVMLifecycle() {
	// Create a test VM in Halted state for start tests
	dynamicClient := dynamic.NewForConfigOrDie(envTestRestConfig)
	vm := &unstructured.Unstructured{}
	vm.SetUnstructuredContent(map[string]interface{}{
		"apiVersion": "kubevirt.io/v1",
		"kind":       "VirtualMachine",
		"metadata": map[string]interface{}{
			"name":      "test-vm-lifecycle",
			"namespace": "default",
		},
		"spec": map[string]interface{}{
			"runStrategy": "Halted",
		},
	})
	_, err := dynamicClient.Resource(schema.GroupVersionResource{
		Group:    "kubevirt.io",
		Version:  "v1",
		Resource: "virtualmachines",
	}).Namespace("default").Create(s.T().Context(), vm, metav1.CreateOptions{})
	s.Require().NoError(err, "failed to create test VM")

	s.Run("vm_lifecycle missing required params", func() {
		testCases := []string{"name", "namespace", "action"}
		for _, param := range testCases {
			s.Run("missing "+param, func() {
				params := map[string]interface{}{
					"name":      "test-vm-lifecycle",
					"namespace": "default",
					"action":    "start",
				}
				delete(params, param)
				toolResult, err := s.CallTool("vm_lifecycle", params)
				s.Require().Nilf(err, "call tool failed %v", err)
				s.Truef(toolResult.IsError, "expected call tool to fail due to missing %s", param)
				s.Equal(toolResult.Content[0].(mcp.TextContent).Text, param+" parameter required")
			})
		}
	})

	s.Run("vm_lifecycle invalid action", func() {
		toolResult, err := s.CallTool("vm_lifecycle", map[string]interface{}{
			"name":      "test-vm-lifecycle",
			"namespace": "default",
			"action":    "invalid",
		})
		s.Require().Nilf(err, "call tool failed %v", err)
		s.Truef(toolResult.IsError, "expected call tool to fail due to invalid action")
		s.Truef(strings.Contains(toolResult.Content[0].(mcp.TextContent).Text, "invalid action"),
			"Expected invalid action message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
	})

	s.Run("vm_lifecycle action=start on halted VM", func() {
		toolResult, err := s.CallTool("vm_lifecycle", map[string]interface{}{
			"name":      "test-vm-lifecycle",
			"namespace": "default",
			"action":    "start",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		var decodedResult []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decodedResult)
		s.Run("returns yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
			s.Truef(strings.HasPrefix(toolResult.Content[0].(mcp.TextContent).Text, "# VirtualMachine started successfully"),
				"Expected success message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
			s.Require().Lenf(decodedResult, 1, "invalid resource count, expected 1, got %v", len(decodedResult))
			s.Equal("test-vm-lifecycle", decodedResult[0].GetName(), "invalid resource name")
			s.Equal("default", decodedResult[0].GetNamespace(), "invalid resource namespace")
			s.Equal("Always",
				decodedResult[0].Object["spec"].(map[string]interface{})["runStrategy"].(string),
				"expected runStrategy to be Always after start")
		})
	})

	s.Run("vm_lifecycle action=start on already running VM (idempotent)", func() {
		toolResult, err := s.CallTool("vm_lifecycle", map[string]interface{}{
			"name":      "test-vm-lifecycle",
			"namespace": "default",
			"action":    "start",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		var decodedResult []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decodedResult)
		s.Run("returns yaml content showing VM was already running", func() {
			s.Nilf(err, "invalid tool result content %v", err)
			expectedPrefix := fmt.Sprintf("# VirtualMachine '%s' in namespace '%s' is already running", "test-vm-lifecycle", "default")
			s.Truef(strings.HasPrefix(toolResult.Content[0].(mcp.TextContent).Text, expectedPrefix),
				"Expected already running message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
			s.Require().Lenf(decodedResult, 1, "invalid resource count, expected 1, got %v", len(decodedResult))
			s.Equal("Always",
				decodedResult[0].Object["spec"].(map[string]interface{})["runStrategy"].(string),
				"expected runStrategy to remain Always")
		})
	})

	s.Run("vm_lifecycle action=stop on running VM", func() {
		toolResult, err := s.CallTool("vm_lifecycle", map[string]interface{}{
			"name":      "test-vm-lifecycle",
			"namespace": "default",
			"action":    "stop",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		var decodedResult []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decodedResult)
		s.Run("returns yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
			s.Truef(strings.HasPrefix(toolResult.Content[0].(mcp.TextContent).Text, "# VirtualMachine stopped successfully"),
				"Expected success message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
			s.Require().Lenf(decodedResult, 1, "invalid resource count, expected 1, got %v", len(decodedResult))
			s.Equal("test-vm-lifecycle", decodedResult[0].GetName(), "invalid resource name")
			s.Equal("default", decodedResult[0].GetNamespace(), "invalid resource namespace")
			s.Equal("Halted",
				decodedResult[0].Object["spec"].(map[string]interface{})["runStrategy"].(string),
				"expected runStrategy to be Halted after stop")
		})
	})

	s.Run("vm_lifecycle action=stop on already stopped VM (idempotent)", func() {
		toolResult, err := s.CallTool("vm_lifecycle", map[string]interface{}{
			"name":      "test-vm-lifecycle",
			"namespace": "default",
			"action":    "stop",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		var decodedResult []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decodedResult)
		s.Run("returns yaml content showing VM was already stopped", func() {
			s.Nilf(err, "invalid tool result content %v", err)
			expectedPrefix := fmt.Sprintf("# VirtualMachine '%s' in namespace '%s' is already stopped", "test-vm-lifecycle", "default")
			s.Truef(strings.HasPrefix(toolResult.Content[0].(mcp.TextContent).Text, expectedPrefix),
				"Expected already stopped message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
			s.Require().Lenf(decodedResult, 1, "invalid resource count, expected 1, got %v", len(decodedResult))
			s.Equal("Halted",
				decodedResult[0].Object["spec"].(map[string]interface{})["runStrategy"].(string),
				"expected runStrategy to remain Halted")
		})
	})

	s.Run("vm_lifecycle action=restart on stopped VM", func() {
		toolResult, err := s.CallTool("vm_lifecycle", map[string]interface{}{
			"name":      "test-vm-lifecycle",
			"namespace": "default",
			"action":    "restart",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		var decodedResult []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decodedResult)
		s.Run("returns yaml content showing VM restarted from stopped state", func() {
			s.Nilf(err, "invalid tool result content %v", err)
			s.Truef(strings.HasPrefix(toolResult.Content[0].(mcp.TextContent).Text, "# VirtualMachine restarted successfully"),
				"Expected success message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
			s.Require().Lenf(decodedResult, 1, "invalid resource count, expected 1, got %v", len(decodedResult))
			s.Equal("Always",
				decodedResult[0].Object["spec"].(map[string]interface{})["runStrategy"].(string),
				"expected runStrategy to be Always after restart from Halted")
		})
	})

	s.Run("vm_lifecycle action=restart on running VM", func() {
		toolResult, err := s.CallTool("vm_lifecycle", map[string]interface{}{
			"name":      "test-vm-lifecycle",
			"namespace": "default",
			"action":    "restart",
		})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		var decodedResult []unstructured.Unstructured
		err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decodedResult)
		s.Run("returns yaml content", func() {
			s.Nilf(err, "invalid tool result content %v", err)
			s.Truef(strings.HasPrefix(toolResult.Content[0].(mcp.TextContent).Text, "# VirtualMachine restarted successfully"),
				"Expected success message, got %v", toolResult.Content[0].(mcp.TextContent).Text)
			s.Require().Lenf(decodedResult, 1, "invalid resource count, expected 1, got %v", len(decodedResult))
			s.Equal("test-vm-lifecycle", decodedResult[0].GetName(), "invalid resource name")
			s.Equal("default", decodedResult[0].GetNamespace(), "invalid resource namespace")
			s.Equal("Always",
				decodedResult[0].Object["spec"].(map[string]interface{})["runStrategy"].(string),
				"expected runStrategy to be Always after restart")
		})
	})

	s.Run("vm_lifecycle on non-existent VM", func() {
		for _, action := range []string{"start", "stop", "restart"} {
			s.Run("action="+action, func() {
				toolResult, err := s.CallTool("vm_lifecycle", map[string]interface{}{
					"name":      "non-existent-vm",
					"namespace": "default",
					"action":    action,
				})
				s.Nilf(err, "call tool failed %v", err)
				s.Truef(toolResult.IsError, "expected call tool to fail for non-existent VM")
				s.Truef(strings.Contains(toolResult.Content[0].(mcp.TextContent).Text, "failed to get VirtualMachine"),
					"Expected error message about VM not found, got %v", toolResult.Content[0].(mcp.TextContent).Text)
			})
		}
	})
}

func (s *KubevirtSuite) TestVMTroubleshootPrompt() {
	s.Run("vm-troubleshoot prompt returns troubleshooting guide", func() {
		result, err := s.GetPrompt(s.T().Context(), mcp.GetPromptRequest{
			Params: mcp.GetPromptParams{
				Name: "vm-troubleshoot",
				Arguments: map[string]string{
					"namespace": "default",
					"name":      "test-vm",
				},
			},
		})

		s.Run("no error", func() {
			s.NoError(err, "GetPrompt failed")
			s.NotNil(result)
		})

		s.Run("returns troubleshooting guide with correct VM details", func() {
			s.Require().NotNil(result)
			s.Require().Len(result.Messages, 2, "Expected 2 messages")

			textContent, ok := result.Messages[0].Content.(mcp.TextContent)
			s.Require().True(ok, "expected TextContent")
			s.Contains(textContent.Text, "# VirtualMachine Troubleshooting Guide")
			s.Contains(textContent.Text, "test-vm")
			s.Contains(textContent.Text, "default")
		})
	})

	s.Run("vm-troubleshoot prompt returns error for missing namespace", func() {
		result, err := s.GetPrompt(s.T().Context(), mcp.GetPromptRequest{
			Params: mcp.GetPromptParams{
				Name: "vm-troubleshoot",
				Arguments: map[string]string{
					"name": "test-vm",
				},
			},
		})
		s.Error(err, "expected error for missing namespace")
		s.Nil(result)
		s.Contains(err.Error(), "namespace")
	})

	s.Run("vm-troubleshoot prompt returns error for missing name", func() {
		result, err := s.GetPrompt(s.T().Context(), mcp.GetPromptRequest{
			Params: mcp.GetPromptParams{
				Name: "vm-troubleshoot",
				Arguments: map[string]string{
					"namespace": "default",
				},
			},
		})
		s.Error(err, "expected error for missing name")
		s.Nil(result)
		s.Contains(err.Error(), "name")
	})
}

func TestKubevirt(t *testing.T) {
	suite.Run(t, new(KubevirtSuite))
}
