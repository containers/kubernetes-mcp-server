package tokenexchange

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"golang.org/x/oauth2"
)

const StrategyKeycloakV2 = "keycloak-v2"

// keycloakV2Exchanger implements Keycloak standard (v2) token exchange.
// Unlike keycloak-v1, it is same-realm only (no subject_issuer) and always
// uses access_token as the subject token type per the Keycloak v2 spec.
type keycloakV2Exchanger struct{}

var _ TokenExchanger = &keycloakV2Exchanger{}

func (e *keycloakV2Exchanger) Exchange(ctx context.Context, cfg *TargetTokenExchangeConfig, subjectToken string) (*oauth2.Token, error) {
	if cfg.SubjectTokenType != "" && cfg.SubjectTokenType != TokenTypeAccessToken {
		klogutil.LogWarn(klogutil.FromContext(ctx), "keycloak-v2 ignores configured subject_token_type and always uses access_token",
			klogutil.Field("configured", cfg.SubjectTokenType),
			klogutil.Field("used", TokenTypeAccessToken),
		)
	}

	data := buildBaseExchangeForm(cfg, subjectToken, TokenTypeAccessToken)

	if cfg.RequestedTokenType != "" {
		data.Set(FormKeyRequestedTokenType, cfg.RequestedTokenType)
	}

	return executeExchange(ctx, cfg, data)
}
