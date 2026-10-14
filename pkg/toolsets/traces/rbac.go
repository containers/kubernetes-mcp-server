package traces

import "github.com/containers/kubernetes-mcp-server/pkg/api"

// rbacTempoDiscovery is the conservative upper bound for Tempo tools that
// discover TempoStack/TempoMonolithic CRs and optionally resolve OpenShift
// Routes. Query tools that resolve a named instance also list those CRs (and
// may read Routes), so they share the same declaration.
func rbacTempoDiscovery() *api.RBACMetadata {
	return api.RBACBounded(
		api.RBACRequirement{
			Verbs:     []string{"list"},
			Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "tempo.grafana.com", Resource: "tempostacks"}},
			Namespace: &api.RBACNamespace{AllNamespaces: true},
		},
		api.RBACRequirement{
			Verbs:     []string{"list"},
			Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "tempo.grafana.com", Resource: "tempomonolithics"}},
			Namespace: &api.RBACNamespace{AllNamespaces: true},
		},
		api.RBACRequirement{
			Verbs:     []string{"get", "list"},
			Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "route.openshift.io", Resource: "routes"}},
			Namespace: &api.RBACNamespace{AllNamespaces: true},
		},
	)
}
