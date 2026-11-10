package kubevirt

import (
	"context"
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

type HCOStatusRetentionSuite struct {
	suite.Suite
}

func TestHCOStatusRetention(t *testing.T) {
	suite.Run(t, new(HCOStatusRetentionSuite))
}

func (s *HCOStatusRetentionSuite) TestComponentListsStopWhenTheBudgetIsFull() {
	ns := "kubevirt-hyperconverged"
	hco := hcoObject(ns, nil)
	kubevirtCR := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kubevirt.io/v1",
		"kind":       "KubeVirt",
		"metadata":   map[string]any{"name": ns, "namespace": ns},
		"status": map[string]any{
			"conditions": []any{map[string]any{
				"type":    "Available",
				"status":  "False",
				"message": strings.Repeat("x", 8000),
			}},
		},
	}}
	var cdiLists, nacLists, eventLists atomic.Int32
	client := newHCODynamicClient(hco, kubevirtCR)
	countList(client, "cdis", &cdiLists)
	countList(client, "networkaddonsconfigs", &nacLists)
	countList(client, "events", &eventLists)

	before := api.NewBuilderWithBudget(0)
	writeHCOBeforeComponents(before, hco, ns)
	limit := int64(len(hcoStatusAssistantText) + before.Len() + 120)
	result, err := hcoStatusHandler(hcoParams(client, limit))
	s.Require().NoError(err)
	text := result.Messages[0].Content.Text
	s.Contains(text, "### KubeVirt: "+ns+"/"+ns)
	s.Contains(text, api.ResultTruncationNotice)
	s.NotContains(text, "## Step 4: Events")
	s.Equal(int32(0), cdiLists.Load())
	s.Equal(int32(0), nacLists.Load())
	s.Equal(int32(0), eventLists.Load())
}

func (s *HCOStatusRetentionSuite) TestWarningEventsStopWhenTheBudgetIsFull() {
	ns := "kubevirt-hyperconverged"
	hco := hcoObject(ns, nil)
	var eventLists atomic.Int32
	client := newHCODynamicClient(hco)
	client.PrependReactor("list", "events", func(clienttesting.Action) (bool, runtime.Object, error) {
		eventLists.Add(1)
		return true, &unstructured.UnstructuredList{Items: []unstructured.Unstructured{
			warningEvent("alpha-event", strings.Repeat("x", 4000)),
			warningEvent("beta-event", strings.Repeat("y", 4000)),
		}}, nil
	})

	before := api.NewBuilderWithBudget(0)
	writeHCOBeforeComponents(before, hco, ns)
	writeManagedComponentStatus(s.T().Context(), client, ns, before)
	step4 := `

---

## Step 4: Events

Review events in the HCO namespace for warnings or errors:

`
	limit := int64(len(hcoStatusAssistantText) + before.Len() + len(step4) + 180)
	result, err := hcoStatusHandler(hcoParams(client, limit))
	s.Require().NoError(err)
	text := result.Messages[0].Content.Text
	s.Equal(int32(1), eventLists.Load())
	s.Contains(text, "alpha-event")
	s.NotContains(text, "beta-event")
	s.Contains(text, api.ResultTruncationNotice)
}

func hcoParams(dyn dynamic.Interface, limit int64) api.PromptHandlerParams {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Version: "v1"}})
	mapper.AddSpecific(
		schema.GroupVersionKind{Version: "v1", Kind: "Event"},
		schema.GroupVersionResource{Version: "v1", Resource: "events"},
		schema.GroupVersionResource{Version: "v1", Resource: "event"},
		meta.RESTScopeNamespace,
	)
	return api.PromptHandlerParams{
		Context: api.WithResultBudget(context.Background(), limit),
		KubernetesClient: hcoClient{
			dyn:    dyn,
			mapper: hcoResettingMapper{RESTMapper: mapper},
		},
	}
}

func hcoObject(namespace string, spec map[string]any) *unstructured.Unstructured {
	if spec == nil {
		spec = map[string]any{}
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "hco.kubevirt.io/v1",
		"kind":       "HyperConverged",
		"metadata":   map[string]any{"name": namespace, "namespace": namespace},
		"spec":       spec,
	}}
}

func warningEvent(name, message string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Event",
		"metadata":   map[string]any{"name": name, "namespace": "kubevirt-hyperconverged"},
		"message":    message,
		"reason":     "Failed",
		"type":       "Warning",
		"involvedObject": map[string]any{
			"kind": "Pod",
			"name": name,
		},
	}}
}

func newHCODynamicClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		kubevirt.HyperConvergedGVR:          "HyperConvergedList",
		kubevirt.KubeVirtCRGVR:              "KubeVirtList",
		kubevirt.CDIGVR:                     "CDIList",
		kubevirt.NetworkAddonsConfigGVR:     "NetworkAddonsConfigList",
		{Version: "v1", Resource: "events"}: "EventList",
	}, objects...)
}

func countList(client *dynamicfake.FakeDynamicClient, resource string, listed *atomic.Int32) {
	client.PrependReactor("list", resource, func(clienttesting.Action) (bool, runtime.Object, error) {
		listed.Add(1)
		return false, nil, nil
	})
}

type hcoResettingMapper struct{ meta.RESTMapper }

func (hcoResettingMapper) Reset() {}

type hcoClient struct {
	kubernetes.Interface
	dyn    dynamic.Interface
	mapper meta.ResettableRESTMapper
}

func (hcoClient) NamespaceOrDefault(ns string) string { return ns }
func (hcoClient) ToRESTConfig() (*rest.Config, error) { return nil, errHCOClientUnused }
func (hcoClient) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	return nil, errHCOClientUnused
}
func (hcoClient) ToRESTMapper() (meta.RESTMapper, error) { return nil, errHCOClientUnused }
func (hcoClient) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return nil
}
func (hcoClient) RESTConfig() *rest.Config                { return nil }
func (c hcoClient) RESTMapper() meta.ResettableRESTMapper { return c.mapper }
func (hcoClient) DiscoveryClient() discovery.CachedDiscoveryInterface {
	return hcoEventDiscovery{}
}
func (hcoClient) RawDiscoveryClient() discovery.DiscoveryInterface { return hcoEventDiscovery{} }
func (c hcoClient) DynamicClient() dynamic.Interface               { return c.dyn }
func (hcoClient) AuthorizationV1() authv1client.AuthorizationV1Interface {
	return nil
}
func (hcoClient) MetricsV1beta1Client() *metricsv1beta1.MetricsV1beta1Client {
	return nil
}

type hcoEventDiscovery struct{ discovery.DiscoveryInterface }

func (hcoEventDiscovery) ServerResourcesForGroupVersion(string) (*metav1.APIResourceList, error) {
	return &metav1.APIResourceList{
		GroupVersion: "v1",
		APIResources: []metav1.APIResource{{Name: "events", Kind: "Event", Namespaced: true}},
	}, nil
}
func (hcoEventDiscovery) Fresh() bool { return true }
func (hcoEventDiscovery) Invalidate() {}

var errHCOClientUnused = errors.New("unused")
