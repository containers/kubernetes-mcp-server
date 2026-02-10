package kubernetes

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

// ResourceValidator validates that resource types (GVK) exist in the cluster.
type ResourceValidator struct {
	restMapperProvider func() meta.RESTMapper
}

// NewResourceValidator creates a new resource validator.
func NewResourceValidator(restMapperProvider func() meta.RESTMapper) *ResourceValidator {
	return &ResourceValidator{
		restMapperProvider: restMapperProvider,
	}
}

func (v *ResourceValidator) Name() string {
	return "resource"
}

func (v *ResourceValidator) Validate(ctx context.Context, req *api.HTTPValidationRequest) error {
	if req.GVK == nil {
		return nil
	}

	restMapper := v.restMapperProvider()
	if restMapper == nil {
		return nil
	}

	_, err := restMapper.RESTMapping(
		schema.GroupKind{Group: req.GVK.Group, Kind: req.GVK.Kind},
		req.GVK.Version,
	)

	if err != nil {
		if meta.IsNoMatchError(err) {
			return api.NewResourceNotFoundError(
				req.GVK.GroupVersion().String(),
				req.GVK.Kind,
			)
		}
		klog.V(4).Infof("RESTMapper error for %v: %v", req.GVK, err)
		return nil
	}

	return nil
}
