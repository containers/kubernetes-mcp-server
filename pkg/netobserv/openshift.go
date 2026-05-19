package netobserv

import (
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/openshift"
	"k8s.io/client-go/discovery"
)

func clusterIsOpenShift(k8s api.KubernetesClient) bool {
	if k8s == nil {
		return false
	}
	return isOpenShiftDiscovery(k8s.DiscoveryClient())
}

func isOpenShiftDiscovery(dc discovery.DiscoveryInterface) bool {
	if dc == nil {
		return false
	}
	return openshift.IsOpenshift(dc)
}
