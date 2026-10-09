package logs

import "github.com/containers/kubernetes-mcp-server/pkg/api"

// rbacLokiDiscovery is the conservative upper bound for Loki tools that discover
// LokiStack CRs and optionally resolve OpenShift Routes for gateway URLs.
// Query tools that resolve a named instance also list LokiStacks (and may read
// Routes), so they share the same declaration.
func rbacLokiDiscovery() *api.RBACMetadata {
	return api.RBACBounded(
		api.RBACRequirement{
			Verbs:     []string{"list"},
			Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "loki.grafana.com", Resource: "lokistacks"}},
			Namespace: &api.RBACNamespace{AllNamespaces: true},
		},
		api.RBACRequirement{
			Verbs:     []string{"get", "list"},
			Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "route.openshift.io", Resource: "routes"}},
			Namespace: &api.RBACNamespace{AllNamespaces: true},
		},
	)
}
