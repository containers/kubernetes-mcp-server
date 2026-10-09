package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	promapi "github.com/prometheus/client_golang/api"
	promcfg "github.com/prometheus/common/config"
	"k8s.io/client-go/rest"
)

// AuthMode defines the authentication mode
type AuthMode string

const (
	// AuthModeKubeConfig reads the bearer token from the kubeconfig or from the mounted service account token file.
	AuthModeKubeConfig AuthMode = "kubeconfig"
	// AuthModeHeader reads the bearer token from the incoming request's authorization header.
	// The caller must store the token in the context.
	AuthModeHeader AuthMode = "header"
)

const (
	serviceCAFile = "/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt"
)

// ParseAuthMode validates and converts a string to AuthMode
func ParseAuthMode(mode string) (AuthMode, error) {
	switch mode {
	case string(AuthModeKubeConfig):
		return AuthModeKubeConfig, nil
	case string(AuthModeHeader):
		return AuthModeHeader, nil
	default:
		return "", fmt.Errorf("invalid auth mode: %q (valid options: %q, %q)", mode, AuthModeKubeConfig, AuthModeHeader)
	}
}

// BuildRoundTripper creates an http.RoundTripper using the configured auth mode.
func BuildRoundTripper(ctx context.Context, restConfig *rest.Config, authMode AuthMode, useTLS, insecure bool) (http.RoundTripper, error) {
	if restConfig == nil {
		return nil, fmt.Errorf("no REST config available")
	}

	// Do not use rest.TransportFor() in kubeconfig mode, because rest.TransportFor() inherits
	// AccessControlRoundTripper from kubernetes-mcp-server, which provides access control for
	// the Kubernetes HTTP API and misinterprets the Prometheus HTTP API endpoints
	// (e.g. /api/v1/label) as Kubernetes endpoints.
	token, err := readToken(ctx, restConfig, authMode)
	if err != nil {
		return nil, err
	}

	return createRoundTripperWithToken(restConfig, token, useTLS, insecure)
}

func createRoundTripperWithToken(restConfig *rest.Config, token string, useTLS, insecure bool) (http.RoundTripper, error) {
	defaultRt, ok := promapi.DefaultRoundTripper.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("unexpected RoundTripper type: %T, expected *http.Transport", promapi.DefaultRoundTripper)
	}
	rt := defaultRt.Clone()

	if !useTLS {
		slog.Warn("Connecting without TLS")
		return rt, nil
	}

	if insecure {
		rt.TLSClientConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true,
		}
	} else {
		certs, err := createCertPoolFromRESTConfig(restConfig)
		if err != nil {
			return nil, err
		}
		rt.TLSClientConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    certs,
		}
	}

	if token != "" {
		return promcfg.NewAuthorizationCredentialsRoundTripper(
			"Bearer", promcfg.NewInlineSecret(token), rt), nil
	}

	return rt, nil
}

// createCertPoolFromRESTConfig creates a cert pool from Kubernetes REST config.
func createCertPoolFromRESTConfig(restConfig *rest.Config) (*x509.CertPool, error) {
	var certPool *x509.CertPool

	// Start with system cert pool if available
	if systemPool, err := x509.SystemCertPool(); err == nil && systemPool != nil {
		certPool = systemPool
	} else {
		certPool = x509.NewCertPool()
	}

	// Try to append cluster CA from REST config
	var caLoaded bool

	// First, try CAData
	if len(restConfig.CAData) > 0 {
		if ok := certPool.AppendCertsFromPEM(restConfig.CAData); ok {
			caLoaded = true
			slog.Debug("Loaded cluster CA from REST config CAData")
		} else {
			slog.Warn("Failed to parse CA certificates from REST config CAData")
		}
	}

	// If CAData wasn't available, try serviceCAFile
	if !caLoaded {
		caPEM, err := os.ReadFile(serviceCAFile)
		if err != nil {
			slog.Warn("Failed to read CA file", "file", serviceCAFile, "error", err)
		} else {
			if ok := certPool.AppendCertsFromPEM(caPEM); ok {
				slog.Debug("Loaded cluster CA from file", "file", serviceCAFile)
			} else {
				slog.Warn("Failed to parse CA certificates from file", "file", serviceCAFile)
			}
		}
	}

	return certPool, nil
}

func ContextWithAuthFromRequest(ctx context.Context, r *http.Request) context.Context {
	authHeader := r.Header.Get(string(kubernetes.OAuthAuthorizationHeader))
	parts := strings.Fields(authHeader)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		ctx = context.WithValue(ctx, kubernetes.OAuthAuthorizationHeader, "Bearer "+parts[1])
	}
	return ctx
}
