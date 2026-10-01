package netobserv

import (
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

var openshiftProjectGVKs = []schema.GroupVersionKind{{
	Group:   "project.openshift.io",
	Version: "v1",
	Kind:    "Project",
}}

// clusterIsOpenShiftFromDiscovery reports whether OpenShift defaults should be used.
// Unknown discovery results use the safer HTTPS defaults so bearer tokens are not sent over HTTP.
func clusterIsOpenShiftFromDiscovery(dc discovery.DiscoveryInterface) bool {
	if dc == nil {
		return true
	}
	has, err := api.HasGVKs(dc, openshiftProjectGVKs)
	return err != nil || has
}
