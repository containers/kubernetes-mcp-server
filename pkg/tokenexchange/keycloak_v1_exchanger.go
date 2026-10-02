package tokenexchange

import (
	"context"

	"golang.org/x/oauth2"
)

// keycloakV1Exchanger implements Keycloak V1 token exchange
type keycloakV1Exchanger struct{}

var _ TokenExchanger = &keycloakV1Exchanger{}

func (e *keycloakV1Exchanger) Exchange(ctx context.Context, cfg *TargetTokenExchangeConfig, subjectToken string) (*oauth2.Token, error) {
	subjectTokenType := cfg.SubjectTokenType
	if subjectTokenType == "" {
		subjectTokenType = TokenTypeAccessToken
	}

	data := buildBaseExchangeForm(cfg, subjectToken, subjectTokenType)

	if cfg.SubjectIssuer != "" {
		data.Set(FormKeySubjectIssuer, cfg.SubjectIssuer)
	}

	return executeExchange(ctx, cfg, data)
}
