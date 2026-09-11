package tokenexchange

import (
	"context"

	"golang.org/x/oauth2"
)

type rfc8693Exchanger struct{}

var _ TokenExchanger = &rfc8693Exchanger{}

func (e *rfc8693Exchanger) Exchange(ctx context.Context, cfg *TargetTokenExchangeConfig, subjectToken string) (*oauth2.Token, error) {
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

	data := buildBaseExchangeForm(cfg, subjectToken, subjectTokenType)
	data.Set(FormKeyRequestedTokenType, requestedTokenType)

	return executeExchange(ctx, cfg, data)
}
