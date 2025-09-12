package kubernetes

import (
	"context"
	"fmt"

	authenticationv1api "k8s.io/api/authentication/v1"
	authorizationv1api "k8s.io/api/authorization/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	authenticationv1 "k8s.io/client-go/kubernetes/typed/authentication/v1"
	authorizationv1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/metrics/pkg/apis/metrics"
	metricsv1beta1api "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned/typed/metrics/v1beta1"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

// AccessControlClientset is a limited clientset delegating interface to the standard kubernetes.Clientset
// Only a limited set of functions are implemented with a single point of access to the kubernetes API where
// apiVersion and kinds are checked for allowed access
type AccessControlClientset struct {
	cfg             *rest.Config
	delegate        kubernetes.Interface
	discoveryClient discovery.DiscoveryInterface
	metricsV1beta1  *metricsv1beta1.MetricsV1beta1Client
	staticConfig    *config.StaticConfig // TODO: maybe just store the denied resource slice
}

func (a *AccessControlClientset) DiscoveryClient() discovery.DiscoveryInterface {
	return a.discoveryClient
}

func (a *AccessControlClientset) Pods(namespace string) (corev1.PodInterface, error) {
	gvk := &schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}
	if !isAllowed(a.staticConfig, gvk) {
		return nil, isNotAllowedError(gvk)
	}
	return a.delegate.CoreV1().Pods(namespace), nil
}

func (a *AccessControlClientset) PodsExec(namespace, name string, podExecOptions *v1.PodExecOptions) (remotecommand.Executor, error) {
	gvk := &schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}
	if !isAllowed(a.staticConfig, gvk) {
		return nil, isNotAllowedError(gvk)
	}
	// Compute URL
	// https://github.com/kubernetes/kubectl/blob/5366de04e168bcbc11f5e340d131a9ca8b7d0df4/pkg/cmd/exec/exec.go#L382-L397
	execRequest := a.delegate.CoreV1().RESTClient().
		Post().
		Resource("pods").
		Namespace(namespace).
		Name(name).
		SubResource("exec")
	execRequest.VersionedParams(podExecOptions, ParameterCodec)
	spdyExec, err := remotecommand.NewSPDYExecutor(a.cfg, "POST", execRequest.URL())
	if err != nil {
		return nil, err
	}
	webSocketExec, err := remotecommand.NewWebSocketExecutor(a.cfg, "GET", execRequest.URL().String())
	if err != nil {
		return nil, err
	}
	return remotecommand.NewFallbackExecutor(webSocketExec, spdyExec, func(err error) bool {
		return httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err)
	})
}

func (a *AccessControlClientset) PodsMetricses(ctx context.Context, namespace, name string, listOptions metav1.ListOptions) (*metrics.PodMetricsList, error) {
	gvk := &schema.GroupVersionKind{Group: metrics.GroupName, Version: metricsv1beta1api.SchemeGroupVersion.Version, Kind: "PodMetrics"}
	if !isAllowed(a.staticConfig, gvk) {
		return nil, isNotAllowedError(gvk)
	}
	versionedMetrics := &metricsv1beta1api.PodMetricsList{}
	var err error
	if name != "" {
		m, err := a.metricsV1beta1.PodMetricses(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get metrics for pod %s/%s: %w", namespace, name, err)
		}
		versionedMetrics.Items = []metricsv1beta1api.PodMetrics{*m}
	} else {
		versionedMetrics, err = a.metricsV1beta1.PodMetricses(namespace).List(ctx, listOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to list pod metrics in namespace %s: %w", namespace, err)
		}
	}
	convertedMetrics := &metrics.PodMetricsList{}
	return convertedMetrics, metricsv1beta1api.Convert_v1beta1_PodMetricsList_To_metrics_PodMetricsList(versionedMetrics, convertedMetrics, nil)
}

func (a *AccessControlClientset) Services(namespace string) (corev1.ServiceInterface, error) {
	gvk := &schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}
	if !isAllowed(a.staticConfig, gvk) {
		return nil, isNotAllowedError(gvk)
	}
	return a.delegate.CoreV1().Services(namespace), nil
}

func (a *AccessControlClientset) SelfSubjectAccessReviews() (authorizationv1.SelfSubjectAccessReviewInterface, error) {
	gvk := &schema.GroupVersionKind{Group: authorizationv1api.GroupName, Version: authorizationv1api.SchemeGroupVersion.Version, Kind: "SelfSubjectAccessReview"}
	if !isAllowed(a.staticConfig, gvk) {
		return nil, isNotAllowedError(gvk)
	}
	return a.delegate.AuthorizationV1().SelfSubjectAccessReviews(), nil
}

// TokenReview returns TokenReviewInterface
func (a *AccessControlClientset) TokenReview() (authenticationv1.TokenReviewInterface, error) {
	gvk := &schema.GroupVersionKind{Group: authenticationv1api.GroupName, Version: authorizationv1api.SchemeGroupVersion.Version, Kind: "TokenReview"}
	if !isAllowed(a.staticConfig, gvk) {
		return nil, isNotAllowedError(gvk)
	}
	return a.delegate.AuthenticationV1().TokenReviews(), nil
}

func NewAccessControlClientset(cfg *rest.Config, staticConfig *config.StaticConfig) (*AccessControlClientset, error) {
	clientSet, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	metricsClient, err := metricsv1beta1.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &AccessControlClientset{
		cfg:             cfg,
		delegate:        clientSet,
		discoveryClient: clientSet.DiscoveryClient,
		metricsV1beta1:  metricsClient,
		staticConfig:    staticConfig,
	}, nil
}
