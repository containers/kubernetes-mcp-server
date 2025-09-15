package kubernetes

import (
	"context"
	"fmt"
	authenticationv1api "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (m *Manager) VerifyToken(ctx context.Context, token, audience string) (*authenticationv1api.UserInfo, []string, error) {
	tokenReviewClient, err := m.accessControlClientSet.TokenReview()
	if err != nil {
		return nil, nil, err
	}
	tokenReview := &authenticationv1api.TokenReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "authentication.k8s.io/v1",
			Kind:       "TokenReview",
		},
		Spec: authenticationv1api.TokenReviewSpec{
			Token:     token,
			Audiences: []string{audience},
		},
	}

	result, err := tokenReviewClient.Create(ctx, tokenReview, metav1.CreateOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create token review: %v", err)
	}

	if !result.Status.Authenticated {
		if result.Status.Error != "" {
			return nil, nil, fmt.Errorf("token authentication failed: %s", result.Status.Error)
		}
		return nil, nil, fmt.Errorf("token authentication failed")
	}

	return &result.Status.User, result.Status.Audiences, nil
}
