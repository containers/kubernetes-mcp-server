package kubernetes

import (
	"encoding/json"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	fake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	kubetesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	metricsv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned/typed/metrics/v1beta1"
)

// resettableRESTMapper wraps a meta.DefaultRESTMapper to satisfy meta.ResettableRESTMapper.
type resettableRESTMapper struct {
	*meta.DefaultRESTMapper
}

func (r *resettableRESTMapper) Reset() {
	// no-op for tests
}

// fakeKubernetesClient is a test-only implementation of api.KubernetesClient
// that uses k8s.io/client-go/dynamic/fake for the dynamic client.
type fakeKubernetesClient struct {
	*fake.Clientset
	dynamicClient   *dynamicfake.FakeDynamicClient
	restMapper      meta.ResettableRESTMapper
	discoveryClient discovery.CachedDiscoveryInterface
	namespace       string
}

var _ api.KubernetesClient = (*fakeKubernetesClient)(nil)

// newFakeKubernetesClient creates a fakeKubernetesClient wired with a dynamic/fake client.
// gvToListKind maps each GVR to its list kind (e.g. {v1 pods} -> "PodList").
// objects are pre-populated runtime.Objects in the fake dynamic client.
func newFakeKubernetesClient(gvToListKind map[schema.GroupVersionResource]string, objects ...runtime.Object) *fakeKubernetesClient {
	scheme := runtime.NewScheme()
	dynClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvToListKind, objects...)

	// Reactor to handle Apply (ApplyPatchType) by creating or updating the object in the tracker
	dynClient.PrependReactor("patch", "*", func(action kubetesting.Action) (bool, runtime.Object, error) {
		patchAction, ok := action.(kubetesting.PatchAction)
		if !ok || patchAction.GetPatchType() != types.ApplyPatchType {
			return false, nil, nil
		}
		obj := &unstructured.Unstructured{}
		if err := json.Unmarshal(patchAction.GetPatch(), &obj.Object); err != nil {
			return true, nil, err
		}
		gvr := patchAction.GetResource()
		// Try to update existing object, if not found create it
		if err := dynClient.Tracker().Update(gvr, obj, patchAction.GetNamespace()); err != nil {
			if err := dynClient.Tracker().Create(gvr, obj, patchAction.GetNamespace()); err != nil {
				return true, nil, err
			}
		}
		return true, obj, nil
	})

	// Build REST mapper from the GVR-to-ListKind mapping
	var groupVersions []schema.GroupVersion
	gvSet := map[schema.GroupVersion]bool{}
	for gvr := range gvToListKind {
		gv := gvr.GroupVersion()
		if !gvSet[gv] {
			groupVersions = append(groupVersions, gv)
			gvSet[gv] = true
		}
	}

	// Build REST mapper and discovery API resources from the GVR-to-ListKind mapping
	mapper := meta.NewDefaultRESTMapper(groupVersions)
	clientSet := fake.NewSimpleClientset()
	apiResourcesByGV := map[string][]metav1.APIResource{}
	for gvr, listKind := range gvToListKind {
		// Derive GVK from GVR: resource "pods" with listKind "PodList" -> kind "Pod"
		kind := listKind
		if len(kind) > 4 && kind[len(kind)-4:] == "List" {
			kind = kind[:len(kind)-4]
		}
		gvk := gvr.GroupVersion().WithKind(kind)
		mapper.Add(gvk, meta.RESTScopeNamespace)
		apiResourcesByGV[gvr.GroupVersion().String()] = append(apiResourcesByGV[gvr.GroupVersion().String()], metav1.APIResource{
			Name:       gvr.Resource,
			Kind:       kind,
			Namespaced: true,
			Verbs:      metav1.Verbs{"get", "list", "watch", "create", "update", "patch", "delete"},
		})
	}

	// Configure fake discovery resources
	var apiResourceLists []*metav1.APIResourceList
	for gv, resources := range apiResourcesByGV {
		apiResourceLists = append(apiResourceLists, &metav1.APIResourceList{
			GroupVersion: gv,
			APIResources: resources,
		})
	}
	clientSet.Resources = apiResourceLists

	cachedDiscovery := memory.NewMemCacheClient(clientSet.Discovery())

	return &fakeKubernetesClient{
		Clientset:       clientSet,
		dynamicClient:   dynClient,
		restMapper:      &resettableRESTMapper{DefaultRESTMapper: mapper},
		discoveryClient: cachedDiscovery,
		namespace:       "default",
	}
}

func (f *fakeKubernetesClient) DynamicClient() dynamic.Interface {
	return f.dynamicClient
}

func (f *fakeKubernetesClient) RESTMapper() meta.ResettableRESTMapper {
	return f.restMapper
}

func (f *fakeKubernetesClient) DiscoveryClient() discovery.CachedDiscoveryInterface {
	return f.discoveryClient
}

func (f *fakeKubernetesClient) RESTConfig() *rest.Config {
	return &rest.Config{}
}

func (f *fakeKubernetesClient) NamespaceOrDefault(namespace string) string {
	if namespace == "" {
		return f.namespace
	}
	return namespace
}

func (f *fakeKubernetesClient) MetricsV1beta1Client() *metricsv1beta1.MetricsV1beta1Client {
	return nil
}

func (f *fakeKubernetesClient) ToRESTConfig() (*rest.Config, error) {
	return f.RESTConfig(), nil
}

func (f *fakeKubernetesClient) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	return f.discoveryClient, nil
}

func (f *fakeKubernetesClient) ToRESTMapper() (meta.RESTMapper, error) {
	return f.restMapper, nil
}

func (f *fakeKubernetesClient) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return clientcmd.NewDefaultClientConfig(clientcmdapi.Config{}, nil)
}
