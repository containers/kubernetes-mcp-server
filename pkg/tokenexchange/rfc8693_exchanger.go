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
	httpClient, err := cfg.HTTPClient()
	if err != nil {
		return nil, fmt.Errorf("failed to acquire http client to talk to IdP for target: %w", err)
	}

	subjectTokenType := cfg.SubjectTokenType
	if subjectTokenType == "" {
		subjectTokenType = TokenTypeAccessToken
	}
	// requested_token_type is OPTIONAL per RFC 8693 section 2.1: when unspecified the
	// issued token type is at the discretion of the authorization server. Send
	// access_token when unset to preserve this server's pre-existing behaviour; some
	// STS deployments require token-type:jwt to signal the AS should mint a fresh
	// signed JWT rather than echo the subject token shape.
	requestedTokenType := cfg.RequestedTokenType
	if requestedTokenType == "" {
		requestedTokenType = TokenTypeAccessToken
	}

	data := url.Values{}
	data.Set(FormKeyGrantType, GrantTypeTokenExchange)
	data.Set(FormKeySubjectToken, subjectToken)
	data.Set(FormKeySubjectTokenType, subjectTokenType)
	data.Set(FormKeyAudience, cfg.Audience)
	data.Set(FormKeyRequestedTokenType, requestedTokenType)

	if len(cfg.Scopes) > 0 {
		data.Set(FormKeyScope, strings.Join(cfg.Scopes, " "))
	}

	headers := http.Header{}
	if err := injectClientAuth(ctx, cfg, data, headers); err != nil {
		return nil, err
	}

	return doTokenExchange(ctx, httpClient, cfg.TokenURL, data, headers)
}
