package kubernetes

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"k8s.io/metrics/pkg/apis/metrics"
	metricsv1beta1api "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"

	"github.com/manusa/kubernetes-mcp-server/pkg/version"
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
)

type PodsTopOptions struct {
	metav1.ListOptions
	AllNamespaces bool
	Namespace     string
	Name          string
}

func (k *Kubernetes) PodsListInAllNamespaces(ctx context.Context, options ResourceListOptions) (runtime.Unstructured, error) {
	return k.ResourcesList(ctx, &schema.GroupVersionKind{
		Group: "", Version: "v1", Kind: "Pod",
	}, "", options)
}

func (k *Kubernetes) PodsListInNamespace(ctx context.Context, namespace string, options ResourceListOptions) (runtime.Unstructured, error) {
	return k.ResourcesList(ctx, &schema.GroupVersionKind{
		Group: "", Version: "v1", Kind: "Pod",
	}, namespace, options)
}

func (k *Kubernetes) PodsGet(ctx context.Context, namespace, name string) (*unstructured.Unstructured, error) {
	return k.ResourcesGet(ctx, &schema.GroupVersionKind{
		Group: "", Version: "v1", Kind: "Pod",
	}, k.NamespaceOrDefault(namespace), name)
}

func (k *Kubernetes) PodsDelete(ctx context.Context, namespace, name string) (string, error) {
	namespace = k.NamespaceOrDefault(namespace)
	pod, err := k.clientSet.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
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
		if sl, _ := k.clientSet.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: managedLabelSelector.String(),
		}); sl != nil {
			for _, svc := range sl.Items {
				_ = k.clientSet.CoreV1().Services(namespace).Delete(ctx, svc.Name, metav1.DeleteOptions{})
			}
		}
	}

	// Delete managed Route
	if isManaged && k.supportsGroupVersion("route.openshift.io/v1") {
		routeResources := k.dynamicClient.
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
		k.clientSet.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (k *Kubernetes) PodsLog(ctx context.Context, namespace, name, container string) (string, error) {
	tailLines := int64(256)
	req := k.clientSet.CoreV1().Pods(k.NamespaceOrDefault(namespace)).GetLogs(name, &v1.PodLogOptions{
		TailLines: &tailLines,
		Container: container,
	})
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

func (k *Kubernetes) PodsRun(ctx context.Context, namespace, name, image string, port int32) ([]*unstructured.Unstructured, error) {
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
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: k.NamespaceOrDefault(namespace), Labels: labels},
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
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: k.NamespaceOrDefault(namespace), Labels: labels},
			Spec: v1.ServiceSpec{
				Selector: labels,
				Type:     v1.ServiceTypeClusterIP,
				Ports:    []v1.ServicePort{{Port: port, TargetPort: intstr.FromInt32(port)}},
			},
		})
	}
	if port > 0 && k.supportsGroupVersion("route.openshift.io/v1") {
		resources = append(resources, &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "route.openshift.io/v1",
				"kind":       "Route",
				"metadata": map[string]interface{}{
					"name":      name,
					"namespace": k.NamespaceOrDefault(namespace),
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
	return k.resourcesCreateOrUpdate(ctx, toCreate)
}

func (k *Kubernetes) PodsTop(ctx context.Context, options PodsTopOptions) (*metrics.PodMetricsList, error) {
	// TODO, maybe move to mcp Tools setup and omit in case metrics aren't available in the target cluster
	if !k.supportsGroupVersion(metrics.GroupName + "/" + metricsv1beta1api.SchemeGroupVersion.Version) {
		return nil, errors.New("metrics API is not available")
	}
	namespace := options.Namespace
	if options.AllNamespaces && namespace == "" {
		namespace = ""
	} else {
		namespace = k.NamespaceOrDefault(namespace)
	}
	metricsClient, err := metricsclientset.NewForConfig(k.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics client: %w", err)
	}
	versionedMetrics := &metricsv1beta1api.PodMetricsList{}
	if options.Name != "" {
		m, err := metricsClient.MetricsV1beta1().PodMetricses(namespace).Get(ctx, options.Name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get metrics for pod %s/%s: %w", namespace, options.Name, err)
		}
		versionedMetrics.Items = []metricsv1beta1api.PodMetrics{*m}
	} else {
		versionedMetrics, err = metricsClient.MetricsV1beta1().PodMetricses(namespace).List(ctx, options.ListOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to list pod metrics in namespace %s: %w", namespace, err)
		}
	}
	convertedMetrics := &metrics.PodMetricsList{}
	return convertedMetrics, metricsv1beta1api.Convert_v1beta1_PodMetricsList_To_metrics_PodMetricsList(versionedMetrics, convertedMetrics, nil)
}

func (k *Kubernetes) PodsExec(ctx context.Context, namespace, name, container string, command []string) (string, error) {
	namespace = k.NamespaceOrDefault(namespace)
	pod, err := k.clientSet.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
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
	executor, err := k.createExecutor(namespace, name, podExecOptions)
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

func (k *Kubernetes) createExecutor(namespace, name string, podExecOptions *v1.PodExecOptions) (remotecommand.Executor, error) {
	// Compute URL
	// https://github.com/kubernetes/kubectl/blob/5366de04e168bcbc11f5e340d131a9ca8b7d0df4/pkg/cmd/exec/exec.go#L382-L397
	req := k.clientSet.CoreV1().RESTClient().
		Post().
		Resource("pods").
		Namespace(namespace).
		Name(name).
		SubResource("exec")
	req.VersionedParams(podExecOptions, k.parameterCodec)
	spdyExec, err := remotecommand.NewSPDYExecutor(k.cfg, "POST", req.URL())
	if err != nil {
		return nil, err
	}
	webSocketExec, err := remotecommand.NewWebSocketExecutor(k.cfg, "GET", req.URL().String())
	if err != nil {
		return nil, err
	}
	return remotecommand.NewFallbackExecutor(webSocketExec, spdyExec, func(err error) bool {
		return httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err)
	})
}
