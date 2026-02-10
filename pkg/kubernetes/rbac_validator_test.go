package kubernetes

import (
	"context"
	"errors"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
	authv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	authv1client "k8s.io/client-go/kubernetes/typed/authorization/v1"
	"k8s.io/client-go/rest"
)

type mockSelfSubjectAccessReviewInterface struct {
	allowed bool
	err     error
}

func (m *mockSelfSubjectAccessReviewInterface) Create(ctx context.Context, review *authv1.SelfSubjectAccessReview, opts metav1.CreateOptions) (*authv1.SelfSubjectAccessReview, error) {
	if m.err != nil {
		return nil, m.err
	}
	review.Status.Allowed = m.allowed
	return review, nil
}

type mockAuthorizationV1Interface struct {
	authv1client.AuthorizationV1Interface
	selfSubjectAccessReview *mockSelfSubjectAccessReviewInterface
}

func (m *mockAuthorizationV1Interface) RESTClient() rest.Interface {
	return nil
}

func (m *mockAuthorizationV1Interface) SelfSubjectAccessReviews() authv1client.SelfSubjectAccessReviewInterface {
	return m.selfSubjectAccessReview
}

type RBACValidatorTestSuite struct {
	suite.Suite
}

func (s *RBACValidatorTestSuite) TestName() {
	v := NewRBACValidator(nil)
	s.Equal("rbac", v.Name())
}

func (s *RBACValidatorTestSuite) TestValidate() {
	testCases := []struct {
		name        string
		req         *api.HTTPValidationRequest
		authClient  authv1client.AuthorizationV1Interface
		expectError bool
		errorCode   api.ValidationErrorCode
	}{
		{
			name:        "nil GVR passes validation",
			req:         &api.HTTPValidationRequest{GVR: nil, Verb: "get"},
			authClient:  nil,
			expectError: false,
		},
		{
			name: "empty verb passes validation",
			req: &api.HTTPValidationRequest{
				GVR:  &schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
				Verb: "",
			},
			authClient:  nil,
			expectError: false,
		},
		{
			name: "nil auth client passes validation",
			req: &api.HTTPValidationRequest{
				GVR:  &schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
				Verb: "get",
			},
			authClient:  nil,
			expectError: false,
		},
		{
			name: "allowed action passes validation",
			req: &api.HTTPValidationRequest{
				GVR:       &schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
				Verb:      "get",
				Namespace: "default",
			},
			authClient: &mockAuthorizationV1Interface{
				selfSubjectAccessReview: &mockSelfSubjectAccessReviewInterface{allowed: true},
			},
			expectError: false,
		},
		{
			name: "denied action fails validation",
			req: &api.HTTPValidationRequest{
				GVR:       &schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"},
				Verb:      "delete",
				Namespace: "kube-system",
			},
			authClient: &mockAuthorizationV1Interface{
				selfSubjectAccessReview: &mockSelfSubjectAccessReviewInterface{allowed: false},
			},
			expectError: true,
			errorCode:   api.ErrorCodePermissionDenied,
		},
		{
			name: "auth client error passes validation",
			req: &api.HTTPValidationRequest{
				GVR:       &schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
				Verb:      "get",
				Namespace: "default",
			},
			authClient: &mockAuthorizationV1Interface{
				selfSubjectAccessReview: &mockSelfSubjectAccessReviewInterface{err: errors.New("connection refused")},
			},
			expectError: false,
		},
		{
			name: "cluster-scoped resource denied",
			req: &api.HTTPValidationRequest{
				GVR:       &schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"},
				Verb:      "delete",
				Namespace: "",
			},
			authClient: &mockAuthorizationV1Interface{
				selfSubjectAccessReview: &mockSelfSubjectAccessReviewInterface{allowed: false},
			},
			expectError: true,
			errorCode:   api.ErrorCodePermissionDenied,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			v := NewRBACValidator(func() authv1client.AuthorizationV1Interface { return tc.authClient })
			err := v.Validate(context.Background(), tc.req)

			if tc.expectError {
				s.Error(err)
				if ve, ok := err.(*api.ValidationError); ok {
					s.Equal(tc.errorCode, ve.Code)
				}
			} else {
				s.NoError(err)
			}
		})
	}
}

func TestRBACValidator(t *testing.T) {
	suite.Run(t, new(RBACValidatorTestSuite))
}
