package otelcol

import "github.com/containers/kubernetes-mcp-server/pkg/api"

// rbacNoKubernetes declares that otelcol tools use embedded component schemas
// and do not call the Kubernetes API during tool execution.
func rbacNoKubernetes() *api.RBACMetadata {
	return &api.RBACMetadata{
		Version: api.RBACVersionV1Alpha1,
		None: &api.NoRBAC{
			Reason: "Uses embedded OpenTelemetry Collector schemas; does not access the Kubernetes API",
		},
	}
}
