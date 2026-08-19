package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/metrics/pkg/apis/metrics"
	metricsv1beta1api "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

func (c *Core) NodesLog(ctx context.Context, name string, query string, tailLines int64) (string, error) {
	// Use the node proxy API to access logs from the kubelet
	// https://kubernetes.io/docs/concepts/cluster-administration/system-logs/#log-query
	// Query paths are relative to the kubelet's log directory (/var/log).
	// The kubelet SecureJoin normalizes the query against that root, so:
	//   - Service logs: use the service name (e.g. "kubelet", "kube-proxy")
	//   - File logs:    use a path relative to /var/log (e.g. "/kubelet.log", not "/var/log/kubelet.log")
	// See: https://github.com/kubernetes/kubernetes/blob/70d3cc9/pkg/kubelet/kubelet_server_journal.go#L231-L232

	if _, err := c.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{}); err != nil {
		return "", fmt.Errorf("failed to get node %s: %w", name, err)
	}

	req := c.CoreV1().RESTClient().
		Get().
		AbsPath("api", "v1", "nodes", name, "proxy", "logs")
	req.Param("query", query)
	// Only forward tailLines for service-style queries.
	// The kubelet rejects options on file queries (see
	// https://github.com/kubernetes/kubernetes/blob/70d3cc9/pkg/kubelet/kubelet_server_journal.go#L229-L230)
	// because tailLines is only supported for service logs.
	if tailLines > 0 && !strings.ContainsAny(query, `/\\`) {
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

func (c *Core) NodesStatsSummary(ctx context.Context, name string) (string, error) {
	// Use the node proxy API to access stats summary from the kubelet
	// https://kubernetes.io/docs/reference/instrumentation/understand-psi-metrics/
	// This endpoint provides CPU, memory, filesystem, and network statistics

	if _, err := c.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{}); err != nil {
		return "", fmt.Errorf("failed to get node %s: %w", name, err)
	}

	result := c.CoreV1().RESTClient().
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

func (c *Core) NodesTop(ctx context.Context, options api.NodesTopOptions) (*metrics.NodeMetricsList, error) {
	// TODO, maybe move to mcp Tools setup and omit in case metrics aren't available in the target cluster
	if !c.supportsGroupVersion(metrics.GroupName + "/" + metricsv1beta1api.SchemeGroupVersion.Version) {
		return nil, errors.New("metrics API is not available")
	}
	versionedMetrics := &metricsv1beta1api.NodeMetricsList{}
	var err error
	if options.Name != "" {
		m, err := c.MetricsV1beta1Client().NodeMetricses().Get(ctx, options.Name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get metrics for node %s: %w", options.Name, err)
		}
		versionedMetrics.Items = []metricsv1beta1api.NodeMetrics{*m}
	} else {
		versionedMetrics, err = c.MetricsV1beta1Client().NodeMetricses().List(ctx, options.ListOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to list node metrics: %w", err)
		}
	}
	convertedMetrics := &metrics.NodeMetricsList{}
	return convertedMetrics, metricsv1beta1api.Convert_v1beta1_NodeMetricsList_To_metrics_NodeMetricsList(versionedMetrics, convertedMetrics, nil)
}
