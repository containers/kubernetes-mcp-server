package testing

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

// FakeRESTMapper is a simple REST mapper implementation for testing
// that implements meta.ResettableRESTMapper
type FakeRESTMapper struct {
	*meta.DefaultRESTMapper
}

// Reset implements the ResettableRESTMapper interface
func (f *FakeRESTMapper) Reset() {
	// Create a new DefaultRESTMapper to reset state
	f.DefaultRESTMapper = meta.NewDefaultRESTMapper([]schema.GroupVersion{})
}

// NewFakeKubernetesClient creates a Kubernetes client for testing with a fake dynamic client.
// This allows tests to use fake clients without needing a real Kubernetes cluster.
//
// Parameters:
//   - scheme: The runtime.Scheme to use for the fake client (typically runtime.NewScheme())
//   - gvrToListKind: A map of GroupVersionResource to list kind names for custom resources
//   - objects: Initial objects to populate the fake client with
//
// Example usage:
//
//	scheme := runtime.NewScheme()
//	gvrToListKind := map[schema.GroupVersionResource]string{
//	    {Group: "kubevirt.io", Version: "v1", Resource: "virtualmachines"}: "VirtualMachineList",
//	}
//	vm := &unstructured.Unstructured{...}
//	k8s := NewFakeKubernetesClient(scheme, gvrToListKind, vm)
func NewFakeKubernetesClient(
	scheme *runtime.Scheme,
	gvrToListKind map[schema.GroupVersionResource]string,
	objects ...runtime.Object,
) *internalk8s.Kubernetes {
	// Create fake dynamic client with custom list kinds
	fakeDynamicClient := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, objects...)

	// Add a reactor to handle Apply operations (server-side apply)
	// The fake client doesn't natively support Apply, so we implement it
	// by converting Apply to Create or Update
	fakeDynamicClient.PrependReactor("patch", "*", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		patchAction, ok := action.(k8stesting.PatchAction)
		if !ok {
			return false, nil, nil
		}

		// Only handle apply patches (server-side apply uses application/apply-patch+yaml)
		if patchAction.GetPatchType() != "application/apply-patch+yaml" {
			return false, nil, nil
		}

		// For Apply operations, we'll simulate by creating/updating the resource
		// Parse the patch data as an unstructured object
		obj := &unstructured.Unstructured{}
		if err := obj.UnmarshalJSON(patchAction.GetPatch()); err != nil {
			return true, nil, err
		}

		// Set the GVR info
		obj.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   patchAction.GetResource().Group,
			Version: patchAction.GetResource().Version,
			Kind:    obj.GetKind(), // Use kind from the object
		})
		obj.SetName(patchAction.GetName())
		obj.SetNamespace(patchAction.GetNamespace())

		// Try to get the existing resource
		gvr := patchAction.GetResource()
		tracker := fakeDynamicClient.Tracker()
		_, getErr := tracker.Get(gvr, patchAction.GetNamespace(), patchAction.GetName())

		if getErr != nil {
			// Resource doesn't exist, create it
			err = tracker.Create(gvr, obj, patchAction.GetNamespace())
			if err != nil {
				return true, nil, err
			}
			return true, obj, nil
		}

		// Resource exists, update it
		err = tracker.Update(gvr, obj, patchAction.GetNamespace())
		if err != nil {
			return true, nil, err
		}
		return true, obj, nil
	})

	// Create a minimal fake discovery client
	// For basic tests, we don't need a fully functional discovery client
	fakeDiscovery := &fakeDiscoveryClient{gvrToListKind: gvrToListKind}
	cachedDiscovery := memory.NewMemCacheClient(fakeDiscovery)

	// Create a basic REST mapper that implements ResettableRESTMapper
	// For most tests, a default REST mapper should suffice
	defaultMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{})
	restMapper := &FakeRESTMapper{DefaultRESTMapper: defaultMapper}

	// Populate the REST mapper with known GVRs from the gvrToListKind map
	for gvr := range gvrToListKind {
		gvk := schema.GroupVersionKind{
			Group:   gvr.Group,
			Version: gvr.Version,
			Kind:    gvrToListKind[gvr][:len(gvrToListKind[gvr])-4], // Remove "List" suffix
		}
		restMapper.Add(gvk, meta.RESTScopeNamespace)
	}

	// Add common KubeVirt resources to REST mapper for tests
	// VirtualMachine is a common resource that tests will try to create
	kubevirtGV := schema.GroupVersion{Group: "kubevirt.io", Version: "v1"}
	restMapper.Add(
		schema.GroupVersionKind{Group: kubevirtGV.Group, Version: kubevirtGV.Version, Kind: "VirtualMachine"},
		meta.RESTScopeNamespace,
	)
	restMapper.Add(
		schema.GroupVersionKind{Group: kubevirtGV.Group, Version: kubevirtGV.Version, Kind: "VirtualMachineInstance"},
		meta.RESTScopeNamespace,
	)

	// Create AccessControlClientset with fake clients
	accessControlClientset := internalk8s.NewAccessControlClientsetForTesting(
		fakeDynamicClient,
		restMapper,
		cachedDiscovery,
	)

	// Create and return Kubernetes instance
	return internalk8s.NewForTesting(accessControlClientset)
}

// fakeDiscoveryClient is a minimal implementation of discovery.DiscoveryInterface
// that satisfies the interface requirements for testing
type fakeDiscoveryClient struct {
	discovery.DiscoveryInterface
	gvrToListKind map[schema.GroupVersionResource]string
}

// Invalidate is required by the CachedDiscoveryInterface
func (f *fakeDiscoveryClient) Invalidate() {
	// No-op for testing
}

// Fresh returns whether the discovery client is fresh
func (f *fakeDiscoveryClient) Fresh() bool {
	return true
}

// ServerGroups returns the list of supported API groups
func (f *fakeDiscoveryClient) ServerGroups() (*metav1.APIGroupList, error) {
	return &metav1.APIGroupList{
		Groups: []metav1.APIGroup{
			{
				Name: "kubevirt.io",
				Versions: []metav1.GroupVersionForDiscovery{
					{GroupVersion: "kubevirt.io/v1", Version: "v1"},
				},
				PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: "kubevirt.io/v1", Version: "v1"},
			},
			{
				Name: "cdi.kubevirt.io",
				Versions: []metav1.GroupVersionForDiscovery{
					{GroupVersion: "cdi.kubevirt.io/v1beta1", Version: "v1beta1"},
				},
				PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: "cdi.kubevirt.io/v1beta1", Version: "v1beta1"},
			},
			{
				Name: "instancetype.kubevirt.io",
				Versions: []metav1.GroupVersionForDiscovery{
					{GroupVersion: "instancetype.kubevirt.io/v1beta1", Version: "v1beta1"},
				},
				PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: "instancetype.kubevirt.io/v1beta1", Version: "v1beta1"},
			},
		},
	}, nil
}

// ServerResourcesForGroupVersion returns the APIResourceList for a specific group/version
func (f *fakeDiscoveryClient) ServerResourcesForGroupVersion(groupVersion string) (*metav1.APIResourceList, error) {
	gv, err := schema.ParseGroupVersion(groupVersion)
	if err != nil {
		return nil, err
	}

	resources := &metav1.APIResourceList{
		GroupVersion: groupVersion,
		APIResources: []metav1.APIResource{},
	}

	// Add VirtualMachine resources for kubevirt.io/v1
	if gv.Group == "kubevirt.io" && gv.Version == "v1" {
		resources.APIResources = append(resources.APIResources,
			metav1.APIResource{
				Name:       "virtualmachines",
				Namespaced: true,
				Kind:       "VirtualMachine",
				Verbs:      metav1.Verbs{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
			metav1.APIResource{
				Name:       "virtualmachineinstances",
				Namespaced: true,
				Kind:       "VirtualMachineInstance",
				Verbs:      metav1.Verbs{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
		)
	}

	// Add resources from gvrToListKind map
	for gvr, listKind := range f.gvrToListKind {
		if gvr.Group == gv.Group && gvr.Version == gv.Version {
			kind := listKind[:len(listKind)-4] // Remove "List" suffix
			resources.APIResources = append(resources.APIResources, metav1.APIResource{
				Name:       gvr.Resource,
				Namespaced: true,
				Kind:       kind,
				Verbs:      metav1.Verbs{"get", "list", "watch", "create", "update", "patch", "delete"},
			})
		}
	}

	return resources, nil
}
