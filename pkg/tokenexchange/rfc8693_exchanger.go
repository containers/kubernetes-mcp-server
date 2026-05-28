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

	// RFC 8693 §2.1 mandates subject_token_type. Default to access_token — the
	// canonical value for an inbound OAuth access token. Cross-realm flows can
	// override to token-type:jwt.
	subjectTokenType := cfg.SubjectTokenType
	if subjectTokenType == "" {
		subjectTokenType = TokenTypeAccessToken
	}
	// Per RFC 8693 §2.1 requested_token_type defaults to access_token when
	// omitted. Some STS deployments require token-type:jwt to signal the AS
	// should mint a fresh signed JWT rather than echo the subject token shape.
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
	if err := injectClientAuth(cfg, data, headers); err != nil {
		return nil, err
	}

	return doTokenExchange(ctx, httpClient, cfg.TokenURL, data, headers)
}
