package troubleshoot

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubevirt"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	authv1client "k8s.io/client-go/kubernetes/typed/authorization/v1"
	"k8s.io/client-go/rest"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/clientcmd"
	metricsv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned/typed/metrics/v1beta1"
)

type RetentionSuite struct {
	suite.Suite
}

func TestRetention(t *testing.T) {
	suite.Run(t, new(RetentionSuite))
}

func (s *RetentionSuite) TestDataVolumeGetsStopWhenTheBudgetIsFull() {
	var gets []string
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	client.PrependReactor("get", "datavolumes", func(action clienttesting.Action) (bool, runtime.Object, error) {
		gets = append(gets, action.(clienttesting.GetAction).GetName())
		return true, &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "cdi.kubevirt.io/v1beta1",
			"kind":       "DataVolume",
			"metadata":   map[string]any{"name": action.(clienttesting.GetAction).GetName()},
			"status":     map[string]any{"phase": strings.Repeat("p", 400)},
		}}, nil
	})
	vm := &unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{
			"dataVolumeTemplates": []any{
				map[string]any{"metadata": map[string]any{"name": "one"}},
				map[string]any{"metadata": map[string]any{"name": "two"}},
			},
		},
	}}
	header := "## DataVolume/PVC Status\n\n"
	b := api.NewBuilderWithBudget(int64(len(header) + 32))
	fetchDataVolumeStatus(s.T().Context(), client, "ns", vm, b)
	s.Equal([]string{"one"}, gets)
	s.True(b.Truncated())
	s.Contains(b.String(), api.ResultTruncationNotice)
}

func (s *RetentionSuite) TestVMTroubleshootPrefixStops() {
	s.Run("does not get DataVolumes once the VirtualMachine section fills the budget", func() {
		var dvGets atomic.Int32
		vm := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "kubevirt.io/v1",
			"kind":       "VirtualMachine",
			"metadata":   map[string]any{"name": "vm", "namespace": "ns"},
			"spec": map[string]any{
				"dataVolumeTemplates": []any{
					map[string]any{"metadata": map[string]any{"name": "disk"}},
				},
			},
			"status": map[string]any{"printableStatus": strings.Repeat("x", 8000)},
		}}
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
			kubevirt.VirtualMachineGVR:                  "VirtualMachineList",
			kubevirt.VirtualMachineInstanceGVR:          "VirtualMachineInstanceList",
			kubevirt.PodGVR:                             "PodList",
			kubevirt.DataVolumeGVR:                      "DataVolumeList",
			kubevirt.VirtualMachineInstanceMigrationGVR: "VirtualMachineInstanceMigrationList",
		}, vm)
		client.PrependReactor("get", "datavolumes", func(clienttesting.Action) (bool, runtime.Object, error) {
			dvGets.Add(1)
			return true, &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "cdi.kubevirt.io/v1beta1",
				"kind":       "DataVolume",
				"metadata":   map[string]any{"name": "disk"},
			}}, nil
		})

		b := api.NewBuilderWithBudget(400)
		writeVMTroubleshootReport(s.T().Context(), nil, client, "ns", "vm", b)

		s.Equal(int32(0), dvGets.Load())
		s.Contains(b.String(), api.ResultTruncationNotice)
		s.NotContains(b.String(), "## DataVolume/PVC Status")
	})

	s.Run("does not format the next pod once the budget is full", func() {
		diags := []map[string]interface{}{
			{"phase": strings.Repeat("x", 4000)},
			{"phase": "Running"},
		}
		b := api.NewBuilderWithBudget(80)
		wrote := writeVirtLauncherPods(b, []string{"pod-a", "pod-b"}, diags, "")

		s.Equal([]string{"pod-a"}, wrote)
		s.NotContains(b.String(), "pod-b")
		s.Contains(b.String(), api.ResultTruncationNotice)
	})
}

func (s *RetentionSuite) TestEventListsStopWhenTheBudgetIsFull() {
	var lists atomic.Int32
	eventGVR := schema.GroupVersionResource{Version: "v1", Resource: "events"}
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		eventGVR: "EventList",
	})
	dyn.PrependReactor("list", "events", func(clienttesting.Action) (bool, runtime.Object, error) {
		lists.Add(1)
		return true, &unstructured.UnstructuredList{Items: []unstructured.Unstructured{{Object: map[string]any{
			"apiVersion":     "v1",
			"kind":           "Event",
			"metadata":       map[string]any{"name": "warn", "namespace": "ns"},
			"message":        strings.Repeat("m", 400),
			"reason":         "Failed",
			"type":           "Warning",
			"involvedObject": map[string]any{"kind": "VirtualMachine", "name": "vm"},
		}}}}, nil
	})
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Version: "v1"}})
	mapper.AddSpecific(
		schema.GroupVersionKind{Version: "v1", Kind: "Event"},
		schema.GroupVersionResource{Version: "v1", Resource: "events"},
		schema.GroupVersionResource{Version: "v1", Resource: "event"},
		meta.RESTScopeNamespace,
	)
	b := api.NewBuilderWithBudget(32)
	fetchEvents(s.T().Context(), eventListClient{dyn: dyn, mapper: resettingMapper{RESTMapper: mapper}}, "ns", "vm", []string{"pod"}, b)
	s.Equal(int32(1), lists.Load())
	s.True(b.Truncated())
	s.Contains(b.String(), api.ResultTruncationNotice)
}

type resettingMapper struct{ meta.RESTMapper }

func (resettingMapper) Reset() {}

type eventListClient struct {
	kubernetes.Interface
	dyn    dynamic.Interface
	mapper meta.ResettableRESTMapper
}

func (eventListClient) NamespaceOrDefault(ns string) string { return ns }
func (eventListClient) ToRESTConfig() (*rest.Config, error) { return nil, errEventClientUnused }
func (eventListClient) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	return nil, errEventClientUnused
}
func (eventListClient) ToRESTMapper() (meta.RESTMapper, error) { return nil, errEventClientUnused }
func (eventListClient) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return nil
}
func (eventListClient) RESTConfig() *rest.Config { return nil }
func (c eventListClient) RESTMapper() meta.ResettableRESTMapper {
	return c.mapper
}
func (eventListClient) DiscoveryClient() discovery.CachedDiscoveryInterface {
	return staticEventDiscovery{}
}
func (c eventListClient) DynamicClient() dynamic.Interface { return c.dyn }
func (eventListClient) AuthorizationV1() authv1client.AuthorizationV1Interface {
	return nil
}
func (eventListClient) MetricsV1beta1Client() *metricsv1beta1.MetricsV1beta1Client {
	return nil
}

type staticEventDiscovery struct{ discovery.DiscoveryInterface }

func (staticEventDiscovery) ServerResourcesForGroupVersion(string) (*metav1.APIResourceList, error) {
	return &metav1.APIResourceList{
		GroupVersion: "v1",
		APIResources: []metav1.APIResource{{Name: "events", Kind: "Event", Namespaced: true}},
	}, nil
}
func (staticEventDiscovery) Fresh() bool { return true }
func (staticEventDiscovery) Invalidate() {}

var errEventClientUnused = errors.New("unused")
