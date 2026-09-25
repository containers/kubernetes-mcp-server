package tools

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kialiclient "github.com/containers/kubernetes-mcp-server/pkg/kiali"
)

// The Kiali tools call the Kiali HTTP API and forward the caller's bearer token. Kiali
// enforces namespace access via the user's Kubernetes RBAC (get/list namespaces) and uses
// the user's token for mutating Istio/Gateway/Inference config and for pod logs. Read
// paths that only need namespace visibility (metrics, traces, graph, mesh status) are
// gated the same way; observability backends (Prometheus, Jaeger) are reached by Kiali
// itself and are not expressible as Kubernetes RBAC.
//
// Where the Kubernetes checks are derivable from the tool arguments (or a finite enum of
// supported kinds), tools declare a conservative bounded upper bound that mirrors the
// operator's kiali-viewer / kiali ClusterRoles with wildcards expanded to concrete
// resources. Mesh-wide status and cluster listing stay unbounded.

const (
	unboundedMeshStatusReason = "Mesh status aggregates control-plane, data-plane, and observability backends through Kiali; " +
		"the Kubernetes API checks Kiali performs cannot be derived from this capability's arguments"
	unboundedListClustersReason = "Mesh cluster listing is resolved by Kiali from its multi-cluster configuration; " +
		"the Kubernetes API checks Kiali performs cannot be derived from this capability's arguments"
)

func resourceReq(verbs []string, apiGroup, resource string, ns *api.RBACNamespace, name *api.RBACResourceName) api.RBACRequirement {
	return api.RBACRequirement{
		Verbs:        verbs,
		Target:       api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: apiGroup, Resource: resource}},
		Namespace:    ns,
		ResourceName: name,
	}
}

func resourceReqs(verbs []string, apiGroup string, resources []string, ns *api.RBACNamespace) []api.RBACRequirement {
	reqs := make([]api.RBACRequirement, 0, len(resources))
	for _, r := range resources {
		reqs = append(reqs, resourceReq(verbs, apiGroup, r, ns, nil))
	}
	return reqs
}

func appendReqs(dst []api.RBACRequirement, extra ...api.RBACRequirement) []api.RBACRequirement {
	return append(dst, extra...)
}

// networking.istio.io / security.istio.io / inference kinds supported by manage_istio_config*.
var (
	istioNetworkingResources = []string{
		"virtualservices", "destinationrules", "gateways", "serviceentries",
		"sidecars", "workloadentries", "workloadgroups", "envoyfilters",
	}
	istioSecurityResources = []string{
		"authorizationpolicies", "peerauthentications", "requestauthentications",
	}
	gatewayAPIResources = []string{
		"gateways", "httproutes", "grpcroutes", "referencegrants",
		"tcproutes", "tlsroutes", "gatewayclasses",
	}
	inferenceResources = []string{"inferencepools"}
)

func istioConfigResourceReqs(verbs []string, ns *api.RBACNamespace) []api.RBACRequirement {
	reqs := resourceReqs(verbs, "networking.istio.io", istioNetworkingResources, ns)
	reqs = appendReqs(reqs, resourceReqs(verbs, "security.istio.io", istioSecurityResources, ns)...)
	reqs = appendReqs(reqs, resourceReqs(verbs, "gateway.networking.k8s.io", gatewayAPIResources, ns)...)
	reqs = appendReqs(reqs, resourceReqs(verbs, "inference.networking.k8s.io", inferenceResources, ns)...)
	return reqs
}

// LogsRBAC mirrors the pods/log access used by kiali_get_logs (plus get/list pods for
// workload→pod resolution).
func LogsRBAC() *api.RBACMetadata {
	ns := &api.RBACNamespace{Argument: "namespace"}
	return api.RBACBounded(
		resourceReq([]string{"get", "list"}, "", "pods", ns, nil),
		resourceReq([]string{"get"}, "", "pods", ns, &api.RBACResourceName{Argument: "name"}),
		api.RBACRequirement{
			Verbs: []string{"get"},
			Target: api.RBACTarget{Resource: &api.RBACResourceTarget{
				Resource:    "pods",
				Subresource: "log",
			}},
			Namespace: ns,
		},
	)
}

// IstioConfigWriteRBAC declares create/patch/delete on every kind manage_istio_config
// can target (finite enum in the tool schema).
func IstioConfigWriteRBAC() *api.RBACMetadata {
	ns := &api.RBACNamespace{Argument: "namespace"}
	reqs := istioConfigResourceReqs([]string{"create", "patch", "delete"}, ns)
	for i := range reqs {
		reqs[i].ResourceName = &api.RBACResourceName{Argument: "object"}
	}
	return api.RBACBounded(reqs...)
}

// IstioConfigReadRBAC declares get/list on every kind manage_istio_config_read can target.
// Namespace is optional (list across accessible namespaces), so AllNamespaces is the
// conservative bound.
func IstioConfigReadRBAC() *api.RBACMetadata {
	ns := &api.RBACNamespace{AllNamespaces: true}
	return api.RBACBounded(istioConfigResourceReqs([]string{"get", "list"}, ns)...)
}

// ResourcesRBAC declares get/list for every resourceType enum value of
// kiali_get_resource_details. On OpenShift, DeploymentConfigs are included.
func ResourcesRBAC(p api.FilteringProvider) *api.RBACMetadata {
	ns := &api.RBACNamespace{AllNamespaces: true}
	reqs := []api.RBACRequirement{
		resourceReq([]string{"get", "list"}, "", "namespaces", nil, nil),
		resourceReq([]string{"get", "list"}, "", "services", ns, nil),
		resourceReq([]string{"get", "list"}, "", "pods", ns, nil),
		resourceReq([]string{"get", "list"}, "", "replicationcontrollers", ns, nil),
	}
	reqs = appendReqs(reqs, resourceReqs([]string{"get", "list"}, "apps",
		[]string{"deployments", "statefulsets", "daemonsets", "replicasets"}, ns)...)
	reqs = appendReqs(reqs, resourceReqs([]string{"get", "list"}, "batch",
		[]string{"jobs", "cronjobs"}, ns)...)
	reqs = appendReqs(reqs, resourceReq([]string{"get", "list"}, "argoproj.io", "applications", ns, nil))
	if kialiclient.IsOpenShiftFromProvider(context.Background(), p) {
		reqs = appendReqs(reqs,
			resourceReq([]string{"get", "list"}, "apps.openshift.io", "deploymentconfigs", ns, nil),
			resourceReq([]string{"get"}, "route.openshift.io", "routes", ns, nil),
		)
	}
	return api.RBACBounded(reqs...)
}

// NamespaceAccessRBAC is the K8s gate Kiali uses for tools that primarily query
// observability backends (metrics, traces, pod performance): get/list namespaces,
// optionally scoped to a namespace name argument.
func NamespaceAccessRBAC(namespaceNameArg string) *api.RBACMetadata {
	reqs := []api.RBACRequirement{
		resourceReq([]string{"get", "list"}, "", "namespaces", nil, nil),
	}
	if namespaceNameArg != "" {
		reqs = append(reqs, resourceReq([]string{"get"}, "", "namespaces", nil,
			&api.RBACResourceName{Argument: namespaceNameArg}))
	}
	return api.RBACBounded(reqs...)
}

// GraphRBAC covers traffic-graph topology reads: namespace access plus the workload/service
// objects Kiali may resolve while building the graph. The namespaces argument is a
// comma-separated list, so AllNamespaces is the conservative bound.
func GraphRBAC(p api.FilteringProvider) *api.RBACMetadata {
	ns := &api.RBACNamespace{AllNamespaces: true}
	reqs := []api.RBACRequirement{
		resourceReq([]string{"get", "list"}, "", "namespaces", nil, nil),
		resourceReq([]string{"get", "list"}, "", "services", ns, nil),
		resourceReq([]string{"get", "list"}, "", "pods", ns, nil),
	}
	reqs = appendReqs(reqs, resourceReqs([]string{"get", "list"}, "apps",
		[]string{"deployments", "statefulsets", "daemonsets", "replicasets"}, ns)...)
	if kialiclient.IsOpenShiftFromProvider(context.Background(), p) {
		reqs = appendReqs(reqs,
			resourceReq([]string{"get", "list"}, "apps.openshift.io", "deploymentconfigs", ns, nil),
		)
	}
	return api.RBACBounded(reqs...)
}

func MeshStatusRBAC() *api.RBACMetadata {
	return api.RBACUnbounded(unboundedMeshStatusReason)
}

func ListClustersRBAC() *api.RBACMetadata {
	return api.RBACUnbounded(unboundedListClustersReason)
}

// PromptServiceTroubleshootRBAC covers logs + Istio config list for the
// service-troubleshoot prompt (namespace/service/workload args, not tool "name").
func PromptServiceTroubleshootRBAC() *api.RBACMetadata {
	ns := &api.RBACNamespace{Argument: "namespace"}
	logs := api.RBACBounded(
		resourceReq([]string{"get", "list"}, "", "pods", ns, nil),
		api.RBACRequirement{
			Verbs: []string{"get"},
			Target: api.RBACTarget{Resource: &api.RBACResourceTarget{
				Resource:    "pods",
				Subresource: "log",
			}},
			Namespace: ns,
		},
	)
	return MergeRBACBounded(logs, IstioConfigReadRBAC(), NamespaceAccessRBAC("namespace"))
}

// MergeRBACBounded combines bounded declarations into one upper bound. Unbounded or
// none inputs are not supported; callers must only pass bounded metadata.
func MergeRBACBounded(parts ...*api.RBACMetadata) *api.RBACMetadata {
	var reqs []api.RBACRequirement
	for _, p := range parts {
		if p == nil || p.Bounded == nil {
			continue
		}
		reqs = append(reqs, p.Bounded.Requirements...)
	}
	return api.RBACBounded(reqs...)
}
