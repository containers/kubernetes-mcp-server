package api

import (
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	metricsv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned/typed/metrics/v1beta1"
)

// ListOptions contains options for listing Kubernetes resources.
type ListOptions struct {
	metav1.ListOptions
	AsTable bool
}

// PodsTopOptions contains options for getting pod metrics.
type PodsTopOptions struct {
	metav1.ListOptions
	AllNamespaces bool
	Namespace     string
	Name          string
}

// NodesTopOptions contains options for getting node metrics.
type NodesTopOptions struct {
	metav1.ListOptions
	Name string
}

// KubernetesClient defines the interface for Kubernetes operations that tool and prompt handlers need.
// This interface abstracts the concrete Kubernetes implementation to allow controlled access to the underlying resource APIs,
// better decoupling, and testability.
type KubernetesClient interface {
	genericclioptions.RESTClientGetter
	kubernetes.Interface
	// NamespaceOrDefault returns the provided namespace or the default configured namespace if empty
	NamespaceOrDefault(namespace string) string
	// RESTConfig returns the REST config used to create clients
	RESTConfig() *rest.Config
	// RESTMapper returns the REST mapper used to map GVK to GVR
	RESTMapper() meta.ResettableRESTMapper
	// DiscoveryClient returns the cached discovery client
	DiscoveryClient() discovery.CachedDiscoveryInterface
	// DynamicClient returns the dynamic client
	DynamicClient() dynamic.Interface
	// MetricsV1beta1Client returns the metrics v1beta1 client
	MetricsV1beta1Client() *metricsv1beta1.MetricsV1beta1Client
}

// HasGVKs checks if all specified GVKs are available using the provided discovery interface.
// Returns (true, nil) if all GVKs are found.
// Returns (false, nil) if any GVK is missing (either the GroupVersion doesn't exist or the Kind is not found).
// Returns (false, err) if discovery fails with a non-404 error.
// Callers should decide how to interpret non-404 errors based on their context.
func HasGVKs(discoveryClient discovery.DiscoveryInterface, gvks []schema.GroupVersionKind) (bool, error) {
	for _, gvk := range gvks {
		resourceList, err := discoveryClient.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
		if err != nil {
			// If the GroupVersion doesn't exist (404), treat as "GVK not found" rather than an error.
			// The discovery client may return either a StatusError with IsNotFound() true,
			// or memory.ErrCacheNotFound when a cached memcache client sees an absent GroupVersion.
			if apierrors.IsNotFound(err) || errors.Is(err, memory.ErrCacheNotFound) {
				return false, nil
			}
			// Other errors (network issues, etc.) are returned to the caller
			return false, err
		}

		found := false
		for _, apiResource := range resourceList.APIResources {
			if apiResource.Kind == gvk.Kind {
				found = true
				break
			}
		}
		if !found {
			return false, nil
		}
	}

	return true, nil
}
