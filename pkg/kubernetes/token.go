package kubernetes

import (
	"context"

	authenticationv1api "k8s.io/api/authentication/v1"
)

type TokenVerifier interface {
	VerifyToken(ctx context.Context, cluster, token, audience string) (*authenticationv1api.UserInfo, []string, error)
}
