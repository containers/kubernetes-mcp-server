package metrics

import "github.com/containers/kubernetes-mcp-server/pkg/api"

// rbacNoKubernetes declares that metrics tools talk to Prometheus/Alertmanager
// over HTTP and do not call the Kubernetes API during tool execution.
func rbacNoKubernetes() *api.RBACMetadata {
	return &api.RBACMetadata{
		Version: api.RBACVersionV1Alpha1,
		None: &api.NoRBAC{
			Reason: "Queries Prometheus/Thanos/Alertmanager over HTTP; does not access the Kubernetes API",
		},
	}
}
