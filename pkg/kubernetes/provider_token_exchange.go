package kubernetes

import (
	"context"
	"net/http"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/coreos/go-oidc/v3/oidc"
)

type tokenExchangingProvider struct {
	provider          Provider
	stsConfigProvider api.StsConfigProvider
	oidcProvider      *oidc.Provider
	httpClient        *http.Client
}

var _ Provider = &tokenExchangingProvider{}

func newTokenExchangingProvider(
	provider Provider,
	stsConfigProvider api.StsConfigProvider,
	oidcProvider *oidc.Provider,
	httpClient *http.Client,
) Provider {
	return &tokenExchangingProvider{
		provider:          provider,
		stsConfigProvider: stsConfigProvider,
		oidcProvider:      oidcProvider,
		httpClient:        httpClient,
	}
}

func (p *tokenExchangingProvider) GetDerivedKubernetes(ctx context.Context, target string) (*Kubernetes, error) {
	ctx = ExchangeTokenInContext(ctx, p.stsConfigProvider, p.oidcProvider, p.httpClient, p.provider, target)
	return p.provider.GetDerivedKubernetes(ctx, target)
}

func (p *tokenExchangingProvider) IsOpenShift(ctx context.Context) bool {
	return p.provider.IsOpenShift(ctx)
}

func (p *tokenExchangingProvider) GetTargets(ctx context.Context) ([]string, error) {
	return p.provider.GetTargets(ctx)
}

func (p *tokenExchangingProvider) GetDefaultTarget() string {
	return p.provider.GetDefaultTarget()
}

func (p *tokenExchangingProvider) GetTargetParameterName() string {
	return p.provider.GetTargetParameterName()
}

func (p *tokenExchangingProvider) WatchTargets(reload McpReload) {
	p.provider.WatchTargets(reload)
}

func (p *tokenExchangingProvider) Close() {
	p.provider.Close()
}
