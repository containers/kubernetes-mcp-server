package tokenexchange

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/oauth2"
)

// keycloakV1Exchanger implements Keycloak V1 token exchange
type keycloakV1Exchanger struct{}

var _ TokenExchanger = &keycloakV1Exchanger{}

func (e *keycloakV1Exchanger) Exchange(ctx context.Context, cfg *TargetTokenExchangeConfig, subjectToken string) (*oauth2.Token, error) {
	httpClient, err := cfg.HTTPCLient()
	if err != nil {
		return nil, fmt.Errorf("failed to acquire http client to talk to IdP for target: %w", err)
	}

	data := url.Values{}
	data.Set(FormKeyGrantType, GrantTypeTokenExchange)
	data.Set(FormKeySubjectToken, subjectToken)
	data.Set(FormKeySubjectTokenType, cfg.SubjectTokenType)
	data.Set(FormKeyAudience, cfg.Audience)

	if cfg.SubjectIssuer != "" {
		data.Set(FormKeySubjectIssuer, cfg.SubjectIssuer)
	}

	if len(cfg.Scopes) > 0 {
		data.Set(FormKeyScope, strings.Join(cfg.Scopes, " "))
	}

	headers := http.Header{}
	injectClientAuth(cfg, data, headers)

	return doTokenExchange(ctx, httpClient, cfg.TokenURL, data, headers)
}
