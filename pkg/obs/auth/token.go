package auth

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"k8s.io/client-go/rest"
)

func readToken(ctx context.Context, restConfig *rest.Config, authMode AuthMode) (string, error) {
	switch authMode {
	case AuthModeKubeConfig:
		return readTokenFromRestConfig(restConfig)

	case AuthModeHeader:
		// Read token from context.
		// The caller is responsible for putting the token from the request header into the context.
		token := readTokenFromContext(ctx)
		if token == "" {
			slog.Warn("no bearer token found in request context authorization header")
		}
		return token, nil

	default:
		return "", fmt.Errorf("unsupported auth mode: %s", authMode)
	}
}

// readTokenFromRestConfig extracts the bearer token from Kubernetes REST config.
func readTokenFromRestConfig(restConfig *rest.Config) (string, error) {
	if restConfig == nil {
		return "", fmt.Errorf("no REST config available")
	}

	if restConfig.BearerToken != "" {
		return restConfig.BearerToken, nil
	}

	if restConfig.BearerTokenFile != "" {
		token, err := os.ReadFile(restConfig.BearerTokenFile)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(token)), nil
	}

	return "", nil
}

// readTokenFromContext reads a token from the context and strips the Bearer prefix
func readTokenFromContext(ctx context.Context) string {
	authHeader, ok := ctx.Value(kubernetes.OAuthAuthorizationHeader).(string)
	if !ok {
		return ""
	}
	parts := strings.Fields(authHeader)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}
	return strings.TrimSpace(authHeader)
}
