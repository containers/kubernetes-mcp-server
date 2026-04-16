package tokenexchange

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/oauth2"
)

const (
	StrategyEntraOBO = "entra-obo"

	// Entra ID OBO-specific constants
	GrantTypeJWTBearer   = "urn:ietf:params:oauth:grant-type:jwt-bearer"
	FormKeyAssertion     = "assertion"
	FormKeyRequestedUse  = "requested_token_use"
	RequestedTokenUseOBO = "on_behalf_of"
)

// entraOBOExchanger implements the Entra ID On-Behalf-Of flow.
// This is used when the MCP server needs to exchange a user's token for a token
// that can access downstream APIs (like Kubernetes) on behalf of that user.
//
// See: https://learn.microsoft.com/en-us/entra/identity-platform/v2-oauth2-on-behalf-of-flow
type entraOBOExchanger struct{}

var _ TokenExchanger = &entraOBOExchanger{}

func (e *entraOBOExchanger) Exchange(ctx context.Context, cfg *TargetTokenExchangeConfig, subjectToken string) (*oauth2.Token, error) {
	httpClient, err := cfg.HTTPClient()
	if err != nil {
		return nil, err
	}

	data := url.Values{}
	data.Set(FormKeyGrantType, GrantTypeJWTBearer)
	data.Set(FormKeyAssertion, subjectToken)
	data.Set(FormKeyRequestedUse, RequestedTokenUseOBO)

	if len(cfg.Scopes) > 0 {
		data.Set(FormKeyScope, strings.Join(cfg.Scopes, " "))
	} else if cfg.Audience != "" {
		data.Set(FormKeyScope, cfg.Audience)
	}

	headers := make(http.Header)
	if err := injectClientAuth(cfg, data, headers); err != nil {
		return nil, err
	}

	return doTokenExchange(ctx, httpClient, cfg.TokenURL, data, headers)
}
