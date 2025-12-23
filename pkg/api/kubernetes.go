package api

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	metricsv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned/typed/metrics/v1beta1"
)

// Openshift provides OpenShift-specific detection capabilities.
// This interface is used by toolsets to conditionally enable OpenShift-specific tools.
type Openshift interface {
	IsOpenShift(context.Context) bool
}

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
