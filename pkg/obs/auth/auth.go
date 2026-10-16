package auth

import (
	"context"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/tlsutil"
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
func BuildRoundTripper(ctx context.Context, cfg *config.Config, restConfig *rest.Config, authMode AuthMode, useTLS, insecure bool) (http.RoundTripper, error) {
	if cfg == nil {
		return nil, fmt.Errorf("no server config available")
	}
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

	return createRoundTripperWithToken(ctx, cfg, restConfig, token, useTLS, insecure)
}

func createRoundTripperWithToken(ctx context.Context, cfg *config.Config, restConfig *rest.Config, token string, useTLS, insecure bool) (http.RoundTripper, error) {
	if insecure {
		return nil, fmt.Errorf("insecure TLS is not supported for observability backends; configure a trusted CA instead")
	}
	defaultRt, ok := promapi.DefaultRoundTripper.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("unexpected RoundTripper type: %T, expected *http.Transport", promapi.DefaultRoundTripper)
	}
	rt := defaultRt.Clone()

	var tlsOpts []tlsutil.TLSConfigOption
	if useTLS {
		certs, err := createCertPoolFromRESTConfig(restConfig)
		if err != nil {
			return nil, err
		}
		tlsOpts = append(tlsOpts, tlsutil.WithRootCAs(certs))
	} else {
		klogutil.FromContext(ctx).V(1).Info("Connecting to observability backend without TLS")
	}
	var err error
	rt.TLSClientConfig, err = tlsutil.BuildTLSConfig(cfg.TLSMinVersion.Get(), cfg.TLSCipherSuites.Get(), tlsOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to build observability TLS config: %w", err)
	}

	var transport http.RoundTripper = config.NewTLSEnforcingTransport(rt, func() bool { return cfg.RequireTLS.Get() })
	if token != "" {
		transport = promcfg.NewAuthorizationCredentialsRoundTripper(
			"Bearer", promcfg.NewInlineSecret(token), transport)
	}

	return transport, nil
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

	// Use the same CA sources as the Kubernetes REST config, without changing
	// the verification of the backend hostname.
	if len(restConfig.CAData) > 0 {
		if !certPool.AppendCertsFromPEM(restConfig.CAData) {
			return nil, fmt.Errorf("failed to parse CA certificates from REST config CAData")
		}
	} else if restConfig.CAFile != "" {
		caPEM, err := os.ReadFile(restConfig.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read REST config CA file: %w", err)
		}
		if !certPool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("failed to parse REST config CA file %q", restConfig.CAFile)
		}
	} else if _, err := os.Stat(serviceCAFile); err == nil {
		caPEM, err := os.ReadFile(serviceCAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read service CA file: %w", err)
		}
		if !certPool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("failed to parse service CA file %q", serviceCAFile)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to access service CA file: %w", err)
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
