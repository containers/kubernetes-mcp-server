package kubernetes

import (
	"context"
	"fmt"
)

func (k *Kubernetes) NodeLog(ctx context.Context, name string, logPath string, tail int64) (string, error) {
	// Use the node proxy API to access logs from the kubelet
	// Common log paths:
	// - /var/log/kubelet.log - kubelet logs
	// - /var/log/kube-proxy.log - kube-proxy logs
	// - /var/log/containers/ - container logs

	if logPath == "" {
		logPath = "kubelet.log"
	}

	// Build the URL for the node proxy logs endpoint
	url := []string{"api", "v1", "nodes", name, "proxy", "logs", logPath}

	// Query parameters for tail
	params := make(map[string]string)
	if tail > 0 {
		params["tailLines"] = fmt.Sprintf("%d", tail)
	}

	req := k.manager.discoveryClient.RESTClient().
		Get().
		AbsPath(url...)

	// Add tail parameter if specified
	for key, value := range params {
		req.Param(key, value)
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
