package kubernetes

import (
	"bytes"
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/tools/remotecommand"
)

// PodsAttach attaches to a running container in a pod, capturing stdout and
// stderr. Unlike exec, attach connects to the container's primary process
// (PID 1) rather than starting a new command. Mirrors `kubectl attach`.
func (c *Core) PodsAttach(ctx context.Context, namespace, name, container string) (string, string, error) {
	namespace = c.NamespaceOrDefault(namespace)
	pods := c.CoreV1().Pods(namespace)
	pod, err := pods.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", "", err
	}
	if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
		return "", "", fmt.Errorf("cannot attach to a container in a completed pod; current phase is %s", pod.Status.Phase)
	}
	container = resolveContainer(pod, container)

	attachOptions := &v1.PodAttachOptions{
		Container: container,
		Stdout:    true,
		Stderr:    true,
		Stdin:     false,
		TTY:       false,
	}
	attachRequest := c.CoreV1().RESTClient().
		Post().
		Resource("pods").
		Namespace(namespace).
		Name(name).
		SubResource("attach")
	attachRequest.VersionedParams(attachOptions, ParameterCodec)

	restConfig, err := c.ToRESTConfig()
	if err != nil {
		return "", "", err
	}
	spdyExec, err := remotecommand.NewSPDYExecutor(restConfig, "POST", attachRequest.URL())
	if err != nil {
		return "", "", err
	}
	webSocketExec, err := remotecommand.NewWebSocketExecutor(restConfig, "GET", attachRequest.URL().String())
	if err != nil {
		return "", "", err
	}
	executor, err := remotecommand.NewFallbackExecutor(webSocketExec, spdyExec, func(err error) bool {
		return httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err)
	})
	if err != nil {
		return "", "", err
	}
	stdout := bytes.NewBuffer(make([]byte, 0))
	stderr := bytes.NewBuffer(make([]byte, 0))
	if err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: stdout, Stderr: stderr, Tty: false,
	}); err != nil {
		return "", "", err
	}
	return stdout.String(), stderr.String(), nil
}
