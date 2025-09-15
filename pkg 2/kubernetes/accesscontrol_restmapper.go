package kubernetes

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/restmapper"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

type AccessControlRESTMapper struct {
	delegate     *restmapper.DeferredDiscoveryRESTMapper
	staticConfig *config.StaticConfig // TODO: maybe just store the denied resource slice
}

var _ meta.RESTMapper = &AccessControlRESTMapper{}

func (a AccessControlRESTMapper) KindFor(resource schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	gvk, err := a.delegate.KindFor(resource)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}
	if !isAllowed(a.staticConfig, &gvk) {
		return schema.GroupVersionKind{}, isNotAllowedError(&gvk)
	}
	return gvk, nil
}

func (a AccessControlRESTMapper) KindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	gvks, err := a.delegate.KindsFor(resource)
	if err != nil {
		return nil, err
	}
	for i := range gvks {
		if !isAllowed(a.staticConfig, &gvks[i]) {
			return nil, isNotAllowedError(&gvks[i])
		}
	}
	return gvks, nil
}

func (a AccessControlRESTMapper) ResourceFor(input schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	return a.delegate.ResourceFor(input)
}

func (a AccessControlRESTMapper) ResourcesFor(input schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	return a.delegate.ResourcesFor(input)
}

func (a AccessControlRESTMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	for _, version := range versions {
		gvk := &schema.GroupVersionKind{Group: gk.Group, Version: version, Kind: gk.Kind}
		if !isAllowed(a.staticConfig, gvk) {
			return nil, isNotAllowedError(gvk)
		}
	}
	return a.delegate.RESTMapping(gk, versions...)
}

func (a AccessControlRESTMapper) RESTMappings(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error) {
	for _, version := range versions {
		gvk := &schema.GroupVersionKind{Group: gk.Group, Version: version, Kind: gk.Kind}
		if !isAllowed(a.staticConfig, gvk) {
			return nil, isNotAllowedError(gvk)
		}
	}
	return a.delegate.RESTMappings(gk, versions...)
}

func (a AccessControlRESTMapper) ResourceSingularizer(resource string) (singular string, err error) {
	return a.delegate.ResourceSingularizer(resource)
}

func (a AccessControlRESTMapper) Reset() {
	a.delegate.Reset()
}

func NewAccessControlRESTMapper(delegate *restmapper.DeferredDiscoveryRESTMapper, staticConfig *config.StaticConfig) *AccessControlRESTMapper {
	return &AccessControlRESTMapper{delegate: delegate, staticConfig: staticConfig}
}
