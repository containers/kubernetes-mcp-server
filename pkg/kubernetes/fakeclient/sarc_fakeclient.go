package fakeclient

import (
	"context"

	authv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	fakedynamic "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	authorizationv1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	metricsv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned/typed/metrics/v1beta1"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

// FakeSARCKubeClient implements api.KubernetesClient for testing SARC calls,
// embeds kubernetes.Interface.
// The fake client ONLY supports sending SARC calls useful for CanIUse invocations.
// Implemented such that the default is to allow ALL unless using WithDeniedAccess option.
type FakeSARCKubeClient struct {
	kubernetes.Interface // embed interface, most methods return nil
	dynamicClient        dynamic.Interface
	discoveryClient      *FakeDiscoveryClient
	restMapper           *ResettableRESTMapper

	// KnownAccessor holds mapping for known allowed and denials
	// in "<verb>:<apiGroup>:<apiResource>:<namespace>:<resourceName>" format.
	KnownAccessor map[string]bool
}

// ResettableRESTMapper wraps a RESTMapper and adds Reset() method
type ResettableRESTMapper struct {
	meta.RESTMapper
}

func (r *ResettableRESTMapper) Reset() {}

// FakeDiscoveryClient implements discovery.CachedDiscoveryInterface
type FakeDiscoveryClient struct {
	discovery.CachedDiscoveryInterface
	APIResourceLists []*metav1.APIResourceList
}

func (f *FakeDiscoveryClient) ServerResourcesForGroupVersion(groupVersion string) (*metav1.APIResourceList, error) {
	for _, rl := range f.APIResourceLists {
		if rl.GroupVersion == groupVersion {
			return rl, nil
		}
	}
	return &metav1.APIResourceList{GroupVersion: groupVersion}, nil
}

func (f *FakeDiscoveryClient) Invalidate() {}
func (f *FakeDiscoveryClient) Fresh() bool { return true }

// FakeAuthorizationV1Client implements authorizationv1.AuthorizationV1Interface
type FakeAuthorizationV1Client struct {
	authorizationv1.AuthorizationV1Interface
	// KnownAccessor holds mapping for known allowed and denials
	// in "<verb>:<apiGroup>:<apiResource>:<namespace>:<resourceName>" format.
	KnownAccessor map[string]bool
}

func (f *FakeAuthorizationV1Client) SelfSubjectAccessReviews() authorizationv1.SelfSubjectAccessReviewInterface {
	return &FakeSelfSubjectAccessReviews{KnownAccessor: f.KnownAccessor}
}

// FakeSelfSubjectAccessReviews implements authorizationv1.SelfSubjectAccessReviewInterface,
// as this is a fake client the default behaviour on SARC create will return allowed: true,
// for denial specifically set it using withDenyResource.
type FakeSelfSubjectAccessReviews struct {
	authorizationv1.SelfSubjectAccessReviewInterface
	// KnownAccessor holds mapping for known allowed and denials
	// in "<verb>:<apiGroup>:<apiResource>:<namespace>:<resourceName>" format.
	KnownAccessor map[string]bool
}

func (f *FakeSelfSubjectAccessReviews) Create(ctx context.Context, review *authv1.SelfSubjectAccessReview, opts metav1.CreateOptions) (*authv1.SelfSubjectAccessReview, error) {
	// allow ALL by default
	review.Status.Allowed = true

	ra := review.Spec.ResourceAttributes

	// Check keys in order of specificity: exact match first, then more general
	// "verb:group:resource:namespace:name" format
	keysToCheck := []string{
		// exact match
		ra.Verb + ":" + ra.Group + ":" + ra.Resource + ":" + ra.Namespace + ":" + ra.Name,
		// any name in namespace
		ra.Verb + ":" + ra.Group + ":" + ra.Resource + ":" + ra.Namespace + ":",
		// specific name, any namespace
		ra.Verb + ":" + ra.Group + ":" + ra.Resource + "::" + ra.Name,
		// any namespace, any name
		ra.Verb + ":" + ra.Group + ":" + ra.Resource + "::",
	}

	for _, key := range keysToCheck {
		if allowed, ok := f.KnownAccessor[key]; ok {
			review.Status.Allowed = allowed
			return review, nil
		}
	}

	return review, nil
}

// Option is a functional option for configuring FakeKubernetesClient
type Option func(*FakeSARCKubeClient)

// NewFakeSARCKubernetesClient creates a fake kubernetes client for testing
// that can be used in tests to perform SARC CanIUse calls.
func NewFakeSARCKubernetesClient(opts ...Option) *FakeSARCKubeClient {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	apiResourcesList := make([]*metav1.APIResourceList, 0)
	apiGroupResources := make([]*restmapper.APIGroupResources, 0)

	client := &FakeSARCKubeClient{
		dynamicClient:   fakedynamic.NewSimpleDynamicClient(scheme),
		discoveryClient: &FakeDiscoveryClient{APIResourceLists: apiResourcesList},
		restMapper:      &ResettableRESTMapper{RESTMapper: restmapper.NewDiscoveryRESTMapper(apiGroupResources)},
		KnownAccessor:   make(map[string]bool),
	}

	for _, opt := range opts {
		opt(client)
	}
	return client
}

// WithDeniedAccess sets the said resources to allowed: false,
// denial on all resources of all namespaces, unless namespace and name are non-empty.
func WithDeniedAccess(verb, group, resource, namespace, name string) Option {
	return func(c *FakeSARCKubeClient) {
		key := verb + ":" + group + ":" + resource + ":" + namespace + ":" + name
		c.KnownAccessor[key] = false
	}
}

func (f *FakeSARCKubeClient) NamespaceOrDefault(namespace string) string {
	if namespace != "" {
		return namespace
	}

	return "default"
}

func (f *FakeSARCKubeClient) RESTConfig() *rest.Config {
	return &rest.Config{Host: "https://fake-server:6443"}
}

func (f *FakeSARCKubeClient) RESTMapper() meta.ResettableRESTMapper {
	return f.restMapper
}

func (f *FakeSARCKubeClient) DiscoveryClient() discovery.CachedDiscoveryInterface {
	return f.discoveryClient
}

func (f *FakeSARCKubeClient) DynamicClient() dynamic.Interface {
	return f.dynamicClient
}

func (f *FakeSARCKubeClient) MetricsV1beta1Client() *metricsv1beta1.MetricsV1beta1Client {
	return nil
}

func (f *FakeSARCKubeClient) AuthorizationV1() authorizationv1.AuthorizationV1Interface {
	return &FakeAuthorizationV1Client{KnownAccessor: f.KnownAccessor}
}

func (f *FakeSARCKubeClient) ToRESTConfig() (*rest.Config, error) {
	return f.RESTConfig(), nil
}

func (f *FakeSARCKubeClient) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	return f.discoveryClient, nil
}

func (f *FakeSARCKubeClient) ToRESTMapper() (meta.RESTMapper, error) {
	return f.restMapper, nil
}

func (f *FakeSARCKubeClient) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return clientcmd.NewDefaultClientConfig(*clientcmdapi.NewConfig(), nil)
}

var _ api.KubernetesClient = (*FakeSARCKubeClient)(nil)
