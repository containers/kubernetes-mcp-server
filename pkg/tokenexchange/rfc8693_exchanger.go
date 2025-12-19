package tokenexchange

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/oauth2"
)

type rfc8693Exchanger struct{}

var _ TokenExchanger = &rfc8693Exchanger{}

func (e *rfc8693Exchanger) Exchange(ctx context.Context, cfg *TargetTokenExchangeConfig, subjectToken string) (*oauth2.Token, error) {
	httpClient, err := cfg.HTTPCLient()
	if err != nil {
		return nil, fmt.Errorf("failed to acquire http client to talk to IdP for target: %w", err)
	}

	data := url.Values{}
	data.Set(FormKeyGrantType, GrantTypeTokenExchange)
	data.Set(FormKeySubjectToken, subjectToken)
	data.Set(FormKeySubjectTokenType, cfg.SubjectTokenType)
	data.Set(FormKeyAudience, cfg.Audience)
	data.Set(FormKeyRequestedTokenType, TokenTypeAccessToken)

	if len(cfg.Scopes) > 0 {
		data.Set(FormKeyScope, strings.Join(cfg.Scopes, " "))
	}

	headers := http.Header{}
	injectClientAuth(cfg, data, headers)

	return doTokenExchange(ctx, httpClient, cfg.TokenURL, data, headers)
}
