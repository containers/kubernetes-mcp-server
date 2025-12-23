package openshift

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

func IsOpenshift(discoveryClient discovery.DiscoveryInterface) bool {
	_, err := discoveryClient.ServerResourcesForGroupVersion(schema.GroupVersion{
		Group:   "project.openshift.io",
		Version: "v1",
	}.String())
	return err == nil
}
