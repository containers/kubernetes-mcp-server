package kubernetes

import (
	"context"
	"fmt"
)

func (k *Kubernetes) NodesLog(ctx context.Context, name string, logPath string, tail int64) (string, error) {
	// Use the node proxy API to access logs from the kubelet
	// Common log paths:
	// - /var/log/kubelet.log - kubelet logs
	// - /var/log/kube-proxy.log - kube-proxy logs
	// - /var/log/containers/ - container logs

	req, err := k.AccessControlClientset().NodesLogs(ctx, name, logPath)
	if err != nil {
		return "", err
	}

	// Query parameters for tail
	if tail > 0 {
		req.Param("tailLines", fmt.Sprintf("%d", tail))
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
	// This endpoint provides CPU, memory, filesystem, and network statistics

	req, err := k.AccessControlClientset().NodesStatsSummary(ctx, name)
	if err != nil {
		return "", err
	}

	result := req.Do(ctx)
	if result.Error() != nil {
		return "", fmt.Errorf("failed to get node stats summary: %w", result.Error())
	}

	rawData, err := result.Raw()
	if err != nil {
		return "", fmt.Errorf("failed to read node stats summary response: %w", err)
	}

	return string(rawData), nil
}
