package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/metrics/pkg/apis/metrics"
	metricsv1beta1api "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/version"
)

func (k *Kubernetes) NodesLog(ctx context.Context, name string, query string, tailLines int64) (string, error) {
	// Use the node proxy API to access logs from the kubelet
	// https://kubernetes.io/docs/concepts/cluster-administration/system-logs/#log-query
	// Common log paths:
	// - /var/log/kubelet.log - kubelet logs
	// - /var/log/kube-proxy.log - kube-proxy logs
	// - /var/log/containers/ - container logs

	if _, err := k.AccessControlClientset().CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{}); err != nil {
		return "", fmt.Errorf("failed to get node %s: %w", name, err)
	}

	req := k.AccessControlClientset().CoreV1().RESTClient().
		Get().
		AbsPath("api", "v1", "nodes", name, "proxy", "logs")
	req.Param("query", query)
	// Query parameters for tail
	if tailLines > 0 {
		req.Param("tailLines", fmt.Sprintf("%d", tailLines))
	}

	result := req.Do(ctx)
	if result.Error() != nil {
		return "", fmt.Errorf("failed to get node logs: %w", result.Error())
	}

	rawData, err := result.Raw()
	if err != nil {
		return "", fmt.Errorf("failed to read node log response: %w", err)
	}

	return string(rawData), nil
}

func (k *Kubernetes) NodesStatsSummary(ctx context.Context, name string) (string, error) {
	// Use the node proxy API to access stats summary from the kubelet
	// https://kubernetes.io/docs/reference/instrumentation/understand-psi-metrics/
	// This endpoint provides CPU, memory, filesystem, and network statistics

	if _, err := k.AccessControlClientset().CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{}); err != nil {
		return "", fmt.Errorf("failed to get node %s: %w", name, err)
	}

	result := k.AccessControlClientset().CoreV1().RESTClient().
		Get().
		AbsPath("api", "v1", "nodes", name, "proxy", "stats", "summary").
		Do(ctx)
	if result.Error() != nil {
		return "", fmt.Errorf("failed to get node stats summary: %w", result.Error())
	}

	rawData, err := result.Raw()
	if err != nil {
		return "", fmt.Errorf("failed to read node stats summary response: %w", err)
	}

	return string(rawData), nil
}

type NodesTopOptions struct {
	metav1.ListOptions
	Name string
}

func (k *Kubernetes) NodesTop(ctx context.Context, options NodesTopOptions) (*metrics.NodeMetricsList, error) {
	// TODO, maybe move to mcp Tools setup and omit in case metrics aren't available in the target cluster
	if !k.supportsGroupVersion(metrics.GroupName + "/" + metricsv1beta1api.SchemeGroupVersion.Version) {
		return nil, errors.New("metrics API is not available")
	}
	versionedMetrics := &metricsv1beta1api.NodeMetricsList{}
	var err error
	if options.Name != "" {
		m, err := k.AccessControlClientset().MetricsV1beta1Client().NodeMetricses().Get(ctx, options.Name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get metrics for node %s: %w", options.Name, err)
		}
		versionedMetrics.Items = []metricsv1beta1api.NodeMetrics{*m}
	} else {
		versionedMetrics, err = k.AccessControlClientset().MetricsV1beta1Client().NodeMetricses().List(ctx, options.ListOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to list node metrics: %w", err)
		}
	}
	convertedMetrics := &metrics.NodeMetricsList{}
	return convertedMetrics, metricsv1beta1api.Convert_v1beta1_NodeMetricsList_To_metrics_NodeMetricsList(versionedMetrics, convertedMetrics, nil)
}

// NodeFilesOptions contains options for node file operations
type NodeFilesOptions struct {
	NodeName   string
	Operation  string // "put", "get", "list"
	SourcePath string
	DestPath   string
	Namespace  string
	Image      string
	Privileged bool
}

// NodesFiles handles file operations on a node filesystem by creating a privileged pod
func (k *Kubernetes) NodesFiles(ctx context.Context, opts NodeFilesOptions) (string, error) {
	// Set defaults
	if opts.Namespace == "" {
		opts.Namespace = "default"
	}
	if opts.Image == "" {
		opts.Image = "busybox"
	}

	// Create privileged pod for accessing node filesystem
	podName := fmt.Sprintf("node-files-%s", rand.String(5))
	pod := &v1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: opts.Namespace,
			Labels: map[string]string{
				AppKubernetesName:      podName,
				AppKubernetesComponent: "node-files",
				AppKubernetesManagedBy: version.BinaryName,
			},
		},
		Spec: v1.PodSpec{
			NodeName:      opts.NodeName,
			RestartPolicy: v1.RestartPolicyNever,
			Containers: []v1.Container{{
				Name:    "node-files",
				Image:   opts.Image,
				Command: []string{"/bin/sh", "-c", "sleep 3600"},
				SecurityContext: &v1.SecurityContext{
					Privileged: ptr.To(opts.Privileged),
				},
				VolumeMounts: []v1.VolumeMount{{
					Name:      "node-root",
					MountPath: "/host",
				}},
			}},
			Volumes: []v1.Volume{{
				Name: "node-root",
				VolumeSource: v1.VolumeSource{
					HostPath: &v1.HostPathVolumeSource{
						Path: "/",
					},
				},
			}},
		},
	}

	// Create the pod
	pods, err := k.AccessControlClientset().Pods(opts.Namespace)
	if err != nil {
		return "", fmt.Errorf("failed to get pods client: %w", err)
	}

	createdPod, err := pods.Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create pod: %w", err)
	}

	// Ensure pod is deleted after operation
	defer func() {
		deleteCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = pods.Delete(deleteCtx, podName, metav1.DeleteOptions{})
	}()

	// Wait for pod to be ready
	if err := k.waitForPodReady(ctx, opts.Namespace, podName, 2*time.Minute); err != nil {
		return "", fmt.Errorf("pod failed to become ready: %w", err)
	}

	// Perform the requested operation
	var result string
	var opErr error
	switch opts.Operation {
	case "put":
		result, opErr = k.nodeFilesPut(ctx, opts.Namespace, podName, opts.SourcePath, opts.DestPath)
	case "get":
		result, opErr = k.nodeFilesGet(ctx, opts.Namespace, podName, opts.SourcePath, opts.DestPath)
	case "list":
		result, opErr = k.nodeFilesList(ctx, opts.Namespace, podName, opts.SourcePath)
	default:
		return "", fmt.Errorf("unknown operation: %s", opts.Operation)
	}

	_ = createdPod
	return result, opErr
}

// nodeFilesPut copies a file from local filesystem to node filesystem
func (k *Kubernetes) nodeFilesPut(ctx context.Context, namespace, podName, sourcePath, destPath string) (string, error) {
	// Read local file content
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", fmt.Errorf("failed to read source file: %w", err)
	}

	// Create destination directory if needed
	destDir := filepath.Dir(destPath)
	if destDir != "." && destDir != "/" {
		mkdirCmd := []string{"/bin/sh", "-c", fmt.Sprintf("mkdir -p /host%s", destDir)}
		if _, err := k.execInPod(ctx, namespace, podName, mkdirCmd); err != nil {
			return "", fmt.Errorf("failed to create destination directory: %w", err)
		}
	}

	// Write content using cat command
	escapedContent := strings.ReplaceAll(string(content), "'", "'\\''")
	writeCmd := []string{"/bin/sh", "-c", fmt.Sprintf("cat > /host%s << 'EOF'\n%s\nEOF", destPath, escapedContent)}

	if _, err := k.execInPod(ctx, namespace, podName, writeCmd); err != nil {
		return "", fmt.Errorf("failed to write file to node: %w", err)
	}

	return fmt.Sprintf("File successfully copied from %s to node:%s", sourcePath, destPath), nil
}

// nodeFilesGet copies a file from node filesystem to local filesystem
func (k *Kubernetes) nodeFilesGet(ctx context.Context, namespace, podName, sourcePath, destPath string) (string, error) {
	// Read file content from node using cat
	readCmd := []string{"/bin/sh", "-c", fmt.Sprintf("cat /host%s", sourcePath)}
	content, err := k.execInPod(ctx, namespace, podName, readCmd)
	if err != nil {
		return "", fmt.Errorf("failed to read file from node: %w", err)
	}

	// Determine destination path
	if destPath == "" {
		destPath = filepath.Base(sourcePath)
	}

	// Create local destination directory if needed
	destDir := filepath.Dir(destPath)
	if destDir != "." && destDir != "" {
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create local directory: %w", err)
		}
	}

	// Write to local file
	if err := os.WriteFile(destPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write local file: %w", err)
	}

	return fmt.Sprintf("File successfully copied from node:%s to %s", sourcePath, destPath), nil
}

// nodeFilesList lists files in a directory on node filesystem
func (k *Kubernetes) nodeFilesList(ctx context.Context, namespace, podName, path string) (string, error) {
	// List directory contents using ls
	listCmd := []string{"/bin/sh", "-c", fmt.Sprintf("ls -la /host%s", path)}
	output, err := k.execInPod(ctx, namespace, podName, listCmd)
	if err != nil {
		return "", fmt.Errorf("failed to list directory: %w", err)
	}

	return output, nil
}

// execInPod executes a command in the pod and returns the output
func (k *Kubernetes) execInPod(ctx context.Context, namespace, podName string, command []string) (string, error) {
	podExecOptions := &v1.PodExecOptions{
		Container: "node-files",
		Command:   command,
		Stdout:    true,
		Stderr:    true,
	}

	// Compute URL
	execRequest := k.AccessControlClientset().CoreV1().RESTClient().
		Post().
		Resource("pods").
		Namespace(namespace).
		Name(podName).
		SubResource("exec")
	execRequest.VersionedParams(podExecOptions, ParameterCodec)

	spdyExec, err := remotecommand.NewSPDYExecutor(k.AccessControlClientset().cfg, "POST", execRequest.URL())
	if err != nil {
		return "", err
	}
	webSocketExec, err := remotecommand.NewWebSocketExecutor(k.AccessControlClientset().cfg, "GET", execRequest.URL().String())
	if err != nil {
		return "", err
	}
	executor, err := remotecommand.NewFallbackExecutor(webSocketExec, spdyExec, func(err error) bool {
		return httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err)
	})
	if err != nil {
		return "", err
	}

	stdout := &strings.Builder{}
	stderr := &strings.Builder{}

	if err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: stdout,
		Stderr: stderr,
		Tty:    false,
	}); err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("exec error: %s: %w", stderr.String(), err)
		}
		return "", err
	}

	if stderr.Len() > 0 && stdout.Len() == 0 {
		return stderr.String(), nil
	}

	return stdout.String(), nil
}

// waitForPodReady waits for a pod to be ready
func (k *Kubernetes) waitForPodReady(ctx context.Context, namespace, podName string, timeout time.Duration) error {
	pods, err := k.AccessControlClientset().Pods(namespace)
	if err != nil {
		return err
	}

	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for pod to be ready")
		}

		pod, err := pods.Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return err
		}

		// Check if pod is ready
		if pod.Status.Phase == v1.PodRunning {
			for _, condition := range pod.Status.Conditions {
				if condition.Type == v1.PodReady && condition.Status == v1.ConditionTrue {
					return nil
				}
			}
		}

		if pod.Status.Phase == v1.PodFailed {
			return fmt.Errorf("pod failed")
		}

		time.Sleep(2 * time.Second)
	}
}

// Ensure io package is used (if not already imported elsewhere)
var _ = io.Copy
