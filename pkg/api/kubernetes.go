package api

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/metrics/pkg/apis/metrics"
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
	RESTConfig() *rest.Config
	RESTMapper() meta.ResettableRESTMapper
	DiscoveryClient() discovery.CachedDiscoveryInterface
	DynamicClient() dynamic.Interface
	MetricsV1beta1Client() *metricsv1beta1.MetricsV1beta1Client

	// TODO: To be removed in next iteration
	// --- Resource Operations ---

	// ResourcesList lists resources of the specified GroupVersionKind
	ResourcesList(ctx context.Context, gvk *schema.GroupVersionKind, namespace string, options ListOptions) (runtime.Unstructured, error)
	// ResourcesGet retrieves a single resource by name
	ResourcesGet(ctx context.Context, gvk *schema.GroupVersionKind, namespace, name string) (*unstructured.Unstructured, error)
	// ResourcesCreateOrUpdate creates or updates resources from a YAML/JSON string
	ResourcesCreateOrUpdate(ctx context.Context, resource string) ([]*unstructured.Unstructured, error)
	// ResourcesDelete deletes a resource by name
	ResourcesDelete(ctx context.Context, gvk *schema.GroupVersionKind, namespace, name string) error
	// ResourcesScale gets or sets the scale of a resource
	ResourcesScale(ctx context.Context, gvk *schema.GroupVersionKind, namespace, name string, desiredScale int64, shouldScale bool) (*unstructured.Unstructured, error)

	// --- Namespace Operations ---

	// NamespacesList lists all namespaces
	NamespacesList(ctx context.Context, options ListOptions) (runtime.Unstructured, error)
	// ProjectsList lists all OpenShift projects
	ProjectsList(ctx context.Context, options ListOptions) (runtime.Unstructured, error)

	// --- Pod Operations ---

	// PodsListInAllNamespaces lists pods across all namespaces
	PodsListInAllNamespaces(ctx context.Context, options ListOptions) (runtime.Unstructured, error)
	// PodsListInNamespace lists pods in a specific namespace
	PodsListInNamespace(ctx context.Context, namespace string, options ListOptions) (runtime.Unstructured, error)
	// PodsGet retrieves a single pod by name
	PodsGet(ctx context.Context, namespace, name string) (*unstructured.Unstructured, error)
	// PodsDelete deletes a pod and its managed resources
	PodsDelete(ctx context.Context, namespace, name string) (string, error)
	// PodsLog retrieves logs from a pod container
	PodsLog(ctx context.Context, namespace, name, container string, previous bool, tail int64) (string, error)
	// PodsRun creates and runs a new pod with optional service and route
	PodsRun(ctx context.Context, namespace, name, image string, port int32) ([]*unstructured.Unstructured, error)
	// PodsTop retrieves pod metrics
	PodsTop(ctx context.Context, options PodsTopOptions) (*metrics.PodMetricsList, error)
	// PodsExec executes a command in a pod container
	PodsExec(ctx context.Context, namespace, name, container string, command []string) (string, error)

	// --- Node Operations ---

	// NodesLog retrieves logs from a node
	NodesLog(ctx context.Context, name string, query string, tailLines int64) (string, error)
	// NodesStatsSummary retrieves stats summary from a node
	NodesStatsSummary(ctx context.Context, name string) (string, error)
	// NodesTop retrieves node metrics
	NodesTop(ctx context.Context, options NodesTopOptions) (*metrics.NodeMetricsList, error)

	// --- Event Operations ---

	// EventsList lists events in a namespace
	EventsList(ctx context.Context, namespace string) ([]map[string]any, error)

	// --- Configuration Operations ---

	// ConfigurationContextsList returns the list of available context names
	ConfigurationContextsList() (map[string]string, error)
	// ConfigurationContextsDefault returns the current context name
	ConfigurationContextsDefault() (string, error)
	// ConfigurationView returns the kubeconfig content
	ConfigurationView(minify bool) (runtime.Object, error)
}
