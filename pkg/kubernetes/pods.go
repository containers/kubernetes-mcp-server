package kubernetes

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	labelutil "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/metrics/pkg/apis/metrics"
	metricsv1beta1api "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/version"
)

// DefaultTailLines is the default number of lines to retrieve from the end of the logs
const DefaultTailLines = int64(100)

func (c *Core) PodsListInAllNamespaces(ctx context.Context, options api.ListOptions) (runtime.Unstructured, error) {
	return c.ResourcesList(ctx, &schema.GroupVersionKind{
		Group: "", Version: "v1", Kind: "Pod",
	}, "", options)
}

func (c *Core) PodsListInNamespace(ctx context.Context, namespace string, options api.ListOptions) (runtime.Unstructured, error) {
	return c.ResourcesList(ctx, &schema.GroupVersionKind{
		Group: "", Version: "v1", Kind: "Pod",
	}, namespace, options)
}

func (c *Core) PodsGet(ctx context.Context, namespace, name string) (*unstructured.Unstructured, error) {
	return c.ResourcesGet(ctx, &schema.GroupVersionKind{
		Group: "", Version: "v1", Kind: "Pod",
	}, c.NamespaceOrDefault(namespace), name)
}

func (c *Core) PodsDelete(ctx context.Context, namespace, name string) (string, error) {
	namespace = c.NamespaceOrDefault(namespace)
	pod, err := c.ResourcesGet(ctx, &schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}, namespace, name)
	if err != nil {
		return "", err
	}

	isManaged := pod.GetLabels()[AppKubernetesManagedBy] == version.BinaryName
	managedLabelSelector := labelutil.Set{
		AppKubernetesManagedBy: version.BinaryName,
		AppKubernetesName:      pod.GetLabels()[AppKubernetesName],
	}.AsSelector()

	// Delete managed service
	if isManaged {
		services := c.CoreV1().Services(namespace)
		if sl, _ := services.List(ctx, metav1.ListOptions{
			LabelSelector: managedLabelSelector.String(),
		}); sl != nil {
			for _, svc := range sl.Items {
				_ = services.Delete(ctx, svc.Name, metav1.DeleteOptions{})
			}
		}
	}

	// Delete managed Route
	if isManaged && c.supportsGroupVersion("route.openshift.io/v1") {
		routeResources := c.DynamicClient().
			Resource(schema.GroupVersionResource{Group: "route.openshift.io", Version: "v1", Resource: "routes"}).
			Namespace(namespace)
		if rl, _ := routeResources.List(ctx, metav1.ListOptions{
			LabelSelector: managedLabelSelector.String(),
		}); rl != nil {
			for _, route := range rl.Items {
				_ = routeResources.Delete(ctx, route.GetName(), metav1.DeleteOptions{})
			}
		}

	}
	return "Pod deleted successfully",
		c.ResourcesDelete(ctx, &schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}, namespace, name, nil)
}

func (c *Core) PodsLog(ctx context.Context, namespace, name, container string, previous bool, tail int64) (string, error) {
	pods := c.CoreV1().Pods(c.NamespaceOrDefault(namespace))

	logOptions := &v1.PodLogOptions{
		Container: container,
		Previous:  previous,
	}

	// Only set tailLines if a value is provided (non-zero)
	if tail > 0 {
		logOptions.TailLines = &tail
	} else {
		// Default to DefaultTailLines lines when not specified
		logOptions.TailLines = ptr.To(DefaultTailLines)
	}

	req := pods.GetLogs(name, logOptions)
	res := req.Do(ctx)
	if res.Error() != nil {
		return "", res.Error()
	}
	rawData, err := res.Raw()
	if err != nil {
		return "", err
	}
	return string(rawData), nil
}

func (c *Core) PodsRun(ctx context.Context, namespace, name, image string, port int32) ([]*unstructured.Unstructured, error) {
	if name == "" {
		name = version.BinaryName + "-run-" + rand.String(5)
	}
	labels := map[string]string{
		AppKubernetesName:      name,
		AppKubernetesComponent: name,
		AppKubernetesManagedBy: version.BinaryName,
		AppKubernetesPartOf:    version.BinaryName + "-run-sandbox",
	}
	// NewPod
	var resources []any
	pod := &v1.Pod{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: c.NamespaceOrDefault(namespace), Labels: labels},
		Spec: v1.PodSpec{Containers: []v1.Container{{
			Name:            name,
			Image:           image,
			ImagePullPolicy: v1.PullAlways,
		}}},
	}
	resources = append(resources, pod)
	if port > 0 {
		pod.Spec.Containers[0].Ports = []v1.ContainerPort{{ContainerPort: port}}
		resources = append(resources, &v1.Service{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: c.NamespaceOrDefault(namespace), Labels: labels},
			Spec: v1.ServiceSpec{
				Selector: labels,
				Type:     v1.ServiceTypeClusterIP,
				Ports:    []v1.ServicePort{{Port: port, TargetPort: intstr.FromInt32(port)}},
			},
		})
	}
	if port > 0 && c.supportsGroupVersion("route.openshift.io/v1") {
		resources = append(resources, &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "route.openshift.io/v1",
				"kind":       "Route",
				"metadata": map[string]interface{}{
					"name":      name,
					"namespace": c.NamespaceOrDefault(namespace),
					"labels":    labels,
				},
				"spec": map[string]interface{}{
					"to": map[string]interface{}{
						"kind":   "Service",
						"name":   name,
						"weight": 100,
					},
					"port": map[string]interface{}{
						"targetPort": intstr.FromInt32(port),
					},
					"tls": map[string]interface{}{
						"termination":                   "edge",
						"insecureEdgeTerminationPolicy": "Redirect",
					},
				},
			},
		})

	}

	// Convert the objects to Unstructured and reuse resourcesCreateOrUpdate functionality
	converter := runtime.DefaultUnstructuredConverter
	var toCreate []*unstructured.Unstructured
	for _, obj := range resources {
		m, err := converter.ToUnstructured(obj)
		if err != nil {
			return nil, err
		}
		u := &unstructured.Unstructured{}
		if err = converter.FromUnstructured(m, u); err != nil {
			return nil, err
		}
		toCreate = append(toCreate, u)
	}
	return c.resourcesCreateOrUpdate(ctx, toCreate)
}

func (c *Core) PodsTop(ctx context.Context, options api.PodsTopOptions) (*metrics.PodMetricsList, error) {
	// TODO, maybe move to mcp Tools setup and omit in case metrics aren't available in the target cluster
	if !c.supportsGroupVersion(metrics.GroupName + "/" + metricsv1beta1api.SchemeGroupVersion.Version) {
		return nil, errors.New("metrics API is not available")
	}
	namespace := options.Namespace
	if options.AllNamespaces && namespace == "" {
		namespace = ""
	} else {
		namespace = c.NamespaceOrDefault(namespace)
	}
	var err error
	versionedMetrics := &metricsv1beta1api.PodMetricsList{}
	if options.Name != "" {
		m, err := c.MetricsV1beta1Client().PodMetricses(namespace).Get(ctx, options.Name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get metrics for pod %s/%s: %w", namespace, options.Name, err)
		}
		versionedMetrics.Items = []metricsv1beta1api.PodMetrics{*m}
	} else {
		versionedMetrics, err = c.MetricsV1beta1Client().PodMetricses(namespace).List(ctx, options.ListOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to list pod metrics in namespace %s: %w", namespace, err)
		}
	}
	convertedMetrics := &metrics.PodMetricsList{}
	return convertedMetrics, metricsv1beta1api.Convert_v1beta1_PodMetricsList_To_metrics_PodMetricsList(versionedMetrics, convertedMetrics, nil)
}

func (c *Core) PodsExec(ctx context.Context, namespace, name, container string, command []string) (string, error) {
	namespace = c.NamespaceOrDefault(namespace)
	pods := c.CoreV1().Pods(namespace)
	pod, err := pods.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	// https://github.com/kubernetes/kubectl/blob/5366de04e168bcbc11f5e340d131a9ca8b7d0df4/pkg/cmd/exec/exec.go#L350-L352
	if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
		return "", fmt.Errorf("cannot exec into a container in a completed pod; current phase is %s", pod.Status.Phase)
	}
	if container == "" {
		container = pod.Spec.Containers[0].Name
	}
	podExecOptions := &v1.PodExecOptions{
		Container: container,
		Command:   command,
		Stdout:    true,
		Stderr:    true,
	}
	// Compute URL
	// https://github.com/kubernetes/kubectl/blob/5366de04e168bcbc11f5e340d131a9ca8b7d0df4/pkg/cmd/exec/exec.go#L382-L397
	execRequest := c.CoreV1().RESTClient().
		Post().
		Resource("pods").
		Namespace(namespace).
		Name(name).
		SubResource("exec")
	execRequest.VersionedParams(podExecOptions, ParameterCodec)
	restConfig, err := c.ToRESTConfig()
	if err != nil {
		return "", err
	}
	spdyExec, err := remotecommand.NewSPDYExecutor(restConfig, "POST", execRequest.URL())
	if err != nil {
		return "", err
	}
	webSocketExec, err := remotecommand.NewWebSocketExecutor(restConfig, "GET", execRequest.URL().String())
	if err != nil {
		return "", err
	}
	executor, err := remotecommand.NewFallbackExecutor(webSocketExec, spdyExec, func(err error) bool {
		return httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err)
	})
	if err != nil {
		return "", err
	}
	stdout := bytes.NewBuffer(make([]byte, 0))
	stderr := bytes.NewBuffer(make([]byte, 0))
	if err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: stdout, Stderr: stderr, Tty: false,
	}); err != nil {
		return "", err
	}
	if stdout.Len() > 0 {
		return stdout.String(), nil
	}
	if stderr.Len() > 0 {
		return stderr.String(), nil
	}
	return "", nil
}
