package mcp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
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
			s.Equal("test-vm", decodedResult[0].GetName(), "invalid resource name")
			s.Equal("default", decodedResult[0].GetNamespace(), "invalid resource namespace")
			s.NotEmptyf(decodedResult[0].GetUID(), "invalid uid, got %v", decodedResult[0].GetUID())
			s.Equal("quay.io/containerdisks/fedora:latest",
				decodedResult[0].Object["spec"].(map[string]interface{})["template"].(map[string]interface{})["spec"].(map[string]interface{})["volumes"].([]interface{})[0].(map[string]interface{})["containerDisk"].(map[string]interface{})["image"].(string),
				"invalid default image")
			s.Equal("Halted",
				decodedResult[0].Object["spec"].(map[string]interface{})["runStrategy"].(string),
				"invalid default runStrategy")
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
			s.Equal("test-vm-2", decodedResult[0].GetName(), "invalid resource name")
			s.Equal("default", decodedResult[0].GetNamespace(), "invalid resource namespace")
			s.NotEmptyf(decodedResult[0].GetUID(), "invalid uid, got %v", decodedResult[0].GetUID())
			s.Equal("quay.io/containerdisks/ubuntu:24.04",
				decodedResult[0].Object["spec"].(map[string]interface{})["template"].(map[string]interface{})["spec"].(map[string]interface{})["volumes"].([]interface{})[0].(map[string]interface{})["containerDisk"].(map[string]interface{})["image"].(string),
				"invalid image for ubuntu workload")
			s.Equal("Halted",
				decodedResult[0].Object["spec"].(map[string]interface{})["runStrategy"].(string),
				"invalid default runStrategy")
			s.Equal("VirtualMachineClusterInstancetype",
				decodedResult[0].Object["spec"].(map[string]interface{})["instancetype"].(map[string]interface{})["kind"].(string),
				"invalid memory for u1.medium instanceType")
			s.Equal("u1.medium",
				decodedResult[0].Object["spec"].(map[string]interface{})["instancetype"].(map[string]interface{})["name"].(string),
				"invalid cpu cores for u1.medium instanceType")
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
			s.Equal("test-vm-3", decodedResult[0].GetName(), "invalid resource name")
			s.Equal("default", decodedResult[0].GetNamespace(), "invalid resource namespace")
			s.NotEmptyf(decodedResult[0].GetUID(), "invalid uid, got %v", decodedResult[0].GetUID())
			s.Equal("rhel",
				decodedResult[0].Object["spec"].(map[string]interface{})["template"].(map[string]interface{})["spec"].(map[string]interface{})["volumes"].([]interface{})[0].(map[string]interface{})["containerDisk"].(map[string]interface{})["image"].(string),
				"invalid image for rhel workload")
			s.Equal("Halted",
				decodedResult[0].Object["spec"].(map[string]interface{})["runStrategy"].(string),
				"invalid default runStrategy")
			s.Equal("VirtualMachineClusterPreference",
				decodedResult[0].Object["spec"].(map[string]interface{})["preference"].(map[string]interface{})["kind"].(string),
				"invalid preference kind for rhel.9 preference")
			s.Equal("rhel.9",
				decodedResult[0].Object["spec"].(map[string]interface{})["preference"].(map[string]interface{})["name"].(string),
				"invalid preference name for rhel.9 preference")
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
			s.Equal("test-vm-4", decodedResult[0].GetName(), "invalid resource name")
			s.Equal("default", decodedResult[0].GetNamespace(), "invalid resource namespace")
			s.NotEmptyf(decodedResult[0].GetUID(), "invalid uid, got %v", decodedResult[0].GetUID())
			s.Equal("quay.io/myrepo/myimage:v1.0",
				decodedResult[0].Object["spec"].(map[string]interface{})["template"].(map[string]interface{})["spec"].(map[string]interface{})["volumes"].([]interface{})[0].(map[string]interface{})["containerDisk"].(map[string]interface{})["image"].(string),
				"invalid image for custom container disk workload")
			s.Equal("Halted",
				decodedResult[0].Object["spec"].(map[string]interface{})["runStrategy"].(string),
				"invalid default runStrategy")
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
				s.Equal("test-vm-5", decodedResult[0].GetName(), "invalid resource name")
				s.Equal("default", decodedResult[0].GetNamespace(), "invalid resource namespace")
				s.NotEmptyf(decodedResult[0].GetUID(), "invalid uid, got %v", decodedResult[0].GetUID())
				s.Equal("VirtualMachineClusterInstancetype",
					decodedResult[0].Object["spec"].(map[string]interface{})["instancetype"].(map[string]interface{})["kind"].(string),
					"invalid instanceType kind for medium size hint")
				s.Equal("u1.medium",
					decodedResult[0].Object["spec"].(map[string]interface{})["instancetype"].(map[string]interface{})["name"].(string),
					"invalid instanceType name for medium size hint")
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
				s.Equal("test-vm-6", decodedResult[0].GetName(), "invalid resource name")
				s.Equal("default", decodedResult[0].GetNamespace(), "invalid resource namespace")
				s.NotEmptyf(decodedResult[0].GetUID(), "invalid uid, got %v", decodedResult[0].GetUID())
				s.Equal("VirtualMachineClusterInstancetype",
					decodedResult[0].Object["spec"].(map[string]interface{})["instancetype"].(map[string]interface{})["kind"].(string),
					"invalid instanceType kind for large size hint")
				s.Equal("c1.large",
					decodedResult[0].Object["spec"].(map[string]interface{})["instancetype"].(map[string]interface{})["name"].(string),
					"invalid instanceType name for large size hint")
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
				s.Equal("test-vm-7", decodedResult[0].GetName(), "invalid resource name")
				s.Equal("default", decodedResult[0].GetNamespace(), "invalid resource namespace")
				s.NotEmptyf(decodedResult[0].GetUID(), "invalid uid, got %v", decodedResult[0].GetUID())
				_, exists := decodedResult[0].Object["spec"].(map[string]interface{})["instancetype"]
				s.Falsef(exists, "expected no instancetype to be set for xlarge size hint")
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
				s.Equal("test-vm-8", decodedResult[0].GetName(), "invalid resource name")
				s.Equal("default", decodedResult[0].GetNamespace(), "invalid resource namespace")
				s.NotEmptyf(decodedResult[0].GetUID(), "invalid uid, got %v", decodedResult[0].GetUID())
				s.Equal("Halted",
					decodedResult[0].Object["spec"].(map[string]interface{})["runStrategy"].(string),
					"invalid default runStrategy")
				s.Equal("VirtualMachineClusterInstancetype",
					decodedResult[0].Object["spec"].(map[string]interface{})["instancetype"].(map[string]interface{})["kind"].(string),
					"invalid instanceType kind from DataSource default")
				s.Equal("u1.medium",
					decodedResult[0].Object["spec"].(map[string]interface{})["instancetype"].(map[string]interface{})["name"].(string),
					"invalid instanceType name from DataSource default")
				s.Equal("VirtualMachineClusterPreference",
					decodedResult[0].Object["spec"].(map[string]interface{})["preference"].(map[string]interface{})["kind"].(string),
					"invalid preference kind from DataSource default")
				s.Equal("fedora",
					decodedResult[0].Object["spec"].(map[string]interface{})["preference"].(map[string]interface{})["name"].(string),
					"invalid preference name from DataSource default")
				s.Equal("DataSource",
					decodedResult[0].Object["spec"].(map[string]interface{})["dataVolumeTemplates"].([]interface{})[0].(map[string]interface{})["spec"].(map[string]interface{})["sourceRef"].(map[string]interface{})["kind"].(string),
					"invalid data source kind in dataVolumeTemplates")
				s.Equal("fedora",
					decodedResult[0].Object["spec"].(map[string]interface{})["dataVolumeTemplates"].([]interface{})[0].(map[string]interface{})["spec"].(map[string]interface{})["sourceRef"].(map[string]interface{})["name"].(string),
					"invalid data source name in dataVolumeTemplates")
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
				s.Equal("test-vm-9", decodedResult[0].GetName(), "invalid resource name")
				s.Equal("default", decodedResult[0].GetNamespace(), "invalid resource namespace")
				s.NotEmptyf(decodedResult[0].GetUID(), "invalid uid, got %v", decodedResult[0].GetUID())
				s.Equal("Halted",
					decodedResult[0].Object["spec"].(map[string]interface{})["runStrategy"].(string),
					"invalid default runStrategy")
				s.Equal("VirtualMachineClusterPreference",
					decodedResult[0].Object["spec"].(map[string]interface{})["preference"].(map[string]interface{})["kind"].(string),
					"invalid preference kind from DataSource default")
				s.Equal("rhel.9",
					decodedResult[0].Object["spec"].(map[string]interface{})["preference"].(map[string]interface{})["name"].(string),
					"invalid preference name from DataSource default")
				s.Equal("DataSource",
					decodedResult[0].Object["spec"].(map[string]interface{})["dataVolumeTemplates"].([]interface{})[0].(map[string]interface{})["spec"].(map[string]interface{})["sourceRef"].(map[string]interface{})["kind"].(string),
					"invalid data source kind in dataVolumeTemplates")
				s.Equal("rhel9",
					decodedResult[0].Object["spec"].(map[string]interface{})["dataVolumeTemplates"].([]interface{})[0].(map[string]interface{})["spec"].(map[string]interface{})["sourceRef"].(map[string]interface{})["name"].(string),
					"invalid data source name in dataVolumeTemplates")
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
				s.Equal("test-vm-10", decodedResult[0].GetName(), "invalid resource name")
				s.Equal("default", decodedResult[0].GetNamespace(), "invalid resource namespace")
				s.NotEmptyf(decodedResult[0].GetUID(), "invalid uid, got %v", decodedResult[0].GetUID())
				s.Equal("Halted",
					decodedResult[0].Object["spec"].(map[string]interface{})["runStrategy"].(string),
					"invalid default runStrategy")
				s.Equal("VirtualMachineClusterInstancetype",
					decodedResult[0].Object["spec"].(map[string]interface{})["instancetype"].(map[string]interface{})["kind"].(string),
					"invalid instanceType kind for large size hint")
				s.Equal("u1.large",
					decodedResult[0].Object["spec"].(map[string]interface{})["instancetype"].(map[string]interface{})["name"].(string),
					"invalid instanceType name for large size hint")
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
				s.Equal("test-vm-11", decodedResult[0].GetName(), "invalid resource name")
				s.Equal("default", decodedResult[0].GetNamespace(), "invalid resource namespace")
				s.NotEmptyf(decodedResult[0].GetUID(), "invalid uid, got %v", decodedResult[0].GetUID())
				s.Equal("VirtualMachineClusterPreference",
					decodedResult[0].Object["spec"].(map[string]interface{})["preference"].(map[string]interface{})["kind"].(string),
					"invalid preference kind for rhel.9 preference")
				s.Equal("rhel.9",
					decodedResult[0].Object["spec"].(map[string]interface{})["preference"].(map[string]interface{})["name"].(string),
					"invalid preference name for rhel.9 preference")
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
				s.Equal("test-vm-12", decodedResult[0].GetName(), "invalid resource name")
				s.Equal("default", decodedResult[0].GetNamespace(), "invalid resource namespace")
				s.NotEmptyf(decodedResult[0].GetUID(), "invalid uid, got %v", decodedResult[0].GetUID())
				s.Equal("custom.preference",
					decodedResult[0].Object["spec"].(map[string]interface{})["preference"].(map[string]interface{})["name"].(string),
					"invalid preference name for explicit custom.preference")
			})
		})
	})
}

func TestKubevirt(t *testing.T) {
	suite.Run(t, new(KubevirtSuite))
}
