package alertmanagement

import "github.com/containers/kubernetes-mcp-server/pkg/api"

func rbacNoKubernetes() *api.RBACMetadata {
	return &api.RBACMetadata{
		Version: api.RBACVersionV1Alpha1,
		None: &api.NoRBAC{
			Reason: "Calls the monitoring-plugin management API over HTTP; backend enforces caller RBAC",
		},
	}
}
