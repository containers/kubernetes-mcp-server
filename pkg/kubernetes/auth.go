package kubernetes

import (
	"context"

	authv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	authv1client "k8s.io/client-go/kubernetes/typed/authorization/v1"
	"k8s.io/klog/v2"
)

// CanI checks if the current identity can perform verb on resource.
// Uses SelfSubjectAccessReview to pre-check RBAC permissions.
func CanI(
	ctx context.Context,
	authClient authv1client.AuthorizationV1Interface,
	gvr *schema.GroupVersionResource,
	namespace, resourceName, verb string,
) (bool, error) {
	if authClient == nil {
		return true, nil
	}

	accessReview := &authv1.SelfSubjectAccessReview{
		Spec: authv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authv1.ResourceAttributes{
				Namespace: namespace,
				Verb:      verb,
				Group:     gvr.Group,
				Version:   gvr.Version,
				Resource:  gvr.Resource,
				Name:      resourceName,
			},
		},
	}

	response, err := authClient.SelfSubjectAccessReviews().Create(ctx, accessReview, metav1.CreateOptions{})
	if err != nil {
		return false, err
	}

	if klog.V(5).Enabled() {
		if response.Status.Allowed {
			klog.V(5).Infof("RBAC check: allowed %s on %s/%s in %s",
				verb, gvr.Group, gvr.Resource, namespace)
		} else {
			klog.V(5).Infof("RBAC check: denied %s on %s/%s in %s: %s",
				verb, gvr.Group, gvr.Resource, namespace, response.Status.Reason)
		}
	}

	return response.Status.Allowed, nil
}
