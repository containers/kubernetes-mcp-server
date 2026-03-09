package kubernetes

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	authv1client "k8s.io/client-go/kubernetes/typed/authorization/v1"
	"k8s.io/klog/v2"
)

// RBACValidator pre-checks RBAC permissions before execution.
type RBACValidator struct {
	authClientProvider func() authv1client.AuthorizationV1Interface
}

// NewRBACValidator creates a new RBAC validator.
func NewRBACValidator(authClientProvider func() authv1client.AuthorizationV1Interface) *RBACValidator {
	return &RBACValidator{
		authClientProvider: authClientProvider,
	}
}

func (v *RBACValidator) Name() string {
	return "rbac"
}

func (v *RBACValidator) Validate(ctx context.Context, req *api.HTTPValidationRequest) error {
	if req.GVR == nil || req.Verb == "" {
		return nil
	}

	authClient := v.authClientProvider()
	if authClient == nil {
		return nil
	}

	allowed, err := CanI(ctx, authClient, req.GVR, req.Namespace, req.ResourceName, req.Verb)
	if err != nil {
		klog.V(4).Infof("RBAC pre-validation failed with error: %v", err)
		return nil
	}

	if !allowed {
		return api.NewPermissionDeniedError(
			req.Verb,
			api.FormatResourceName(req.GVR),
			req.Namespace,
		)
	}

	return nil
}
