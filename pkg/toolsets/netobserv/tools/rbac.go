package tools

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	netobservclient "github.com/containers/kubernetes-mcp-server/pkg/netobserv"
)

// The NetObserv tools query the console plugin backend over HTTP and forward the caller's
// bearer token; the plugin performs its own SubjectAccessReview against that token. On
// OpenShift the plugin uses the NetObserv multi-tenancy model, so the required permissions
// are the operator's reader ClusterRoles and can be declared as a bounded upper bound:
//
//   - netobserv-loki-reader  -> get network.loki.grafana.com (resourceName "logs")  [flows, export]
//   - netobserv-metrics-reader -> create pods.metrics.k8s.io                         [metrics]
//
// On non-OpenShift clusters (or direct-Loki deployments) the plugin's auth backend is not
// derivable here, so the declaration falls back to unbounded.

const (
	unboundedFlowsReason = "Flow records are read through the NetObserv console plugin, which queries Loki and " +
		"runs its own SubjectAccessReview against the forwarded token; the required permissions depend on the plugin's " +
		"Loki backend (direct Loki vs LokiStack gateway multi-tenancy) and cannot be derived from this capability's arguments"
	unboundedMetricsReason = "Flow metrics are read through the NetObserv console plugin, which queries Prometheus and " +
		"runs its own authorization against the forwarded token; the required permissions depend on the plugin's metrics " +
		"backend configuration and cannot be derived from this capability's arguments"
)

// flowsRBAC declares the RBAC for the Loki-backed flow tools (list, export).
// On OpenShift this mirrors the netobserv-loki-reader ClusterRole; otherwise it is unbounded.
func flowsRBAC(p api.FilteringProvider) *api.RBACMetadata {
	if netobservclient.IsOpenShiftFromProvider(context.Background(), p) {
		return api.RBACBounded(api.RBACRequirement{
			Verbs: []string{"get"},
			Target: api.RBACTarget{Resource: &api.RBACResourceTarget{
				APIGroup: "loki.grafana.com",
				Resource: "network",
			}},
			ResourceName: &api.RBACResourceName{Name: "logs"},
		})
	}
	return api.RBACUnbounded(unboundedFlowsReason)
}

// metricsRBAC declares the RBAC for the Prometheus-backed flow metrics tool.
// On OpenShift this mirrors the netobserv-metrics-reader ClusterRole; otherwise it is unbounded.
func metricsRBAC(p api.FilteringProvider) *api.RBACMetadata {
	if netobservclient.IsOpenShiftFromProvider(context.Background(), p) {
		return api.RBACBounded(api.RBACRequirement{
			Verbs: []string{"create"},
			Target: api.RBACTarget{Resource: &api.RBACResourceTarget{
				APIGroup: "metrics.k8s.io",
				Resource: "pods",
			}},
		})
	}
	return api.RBACUnbounded(unboundedMetricsReason)
}
