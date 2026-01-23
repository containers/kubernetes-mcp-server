package kubernetes

import (
	"context"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google/externalaccount"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

type staticSubjectTokenSupplier struct {
	token string
}

func (s *staticSubjectTokenSupplier) SubjectToken(_ context.Context, _ externalaccount.SupplierOptions) (string, error) {
	return s.token, nil
}

var _ externalaccount.SubjectTokenSupplier = &staticSubjectTokenSupplier{}

type SecurityTokenService struct {
	*oidc.Provider
	ClientId                string
	ClientSecret            string
	ExternalAccountAudience string
	ExternalAccountScopes   []string
}

func NewFromConfig(stsConfigProvider api.StsConfigProvider, provider *oidc.Provider) *SecurityTokenService {
	return &SecurityTokenService{
		Provider:                provider,
		ClientId:                stsConfigProvider.GetStsClientId(),
		ClientSecret:            stsConfigProvider.GetStsClientSecret(),
		ExternalAccountAudience: stsConfigProvider.GetStsAudience(),
		ExternalAccountScopes:   stsConfigProvider.GetStsScopes(),
	}
}

func (sts *SecurityTokenService) IsEnabled() bool {
	return sts.Provider != nil && sts.ClientId != "" && sts.ExternalAccountAudience != ""
}

func (sts *SecurityTokenService) ExternalAccountTokenExchange(ctx context.Context, originalToken *oauth2.Token) (*oauth2.Token, error) {
	ts, err := externalaccount.NewTokenSource(ctx, externalaccount.Config{
		TokenURL:             sts.Endpoint().TokenURL,
		ClientID:             sts.ClientId,
		ClientSecret:         sts.ClientSecret,
		Audience:             sts.ExternalAccountAudience,
		SubjectTokenType:     "urn:ietf:params:oauth:token-type:access_token",
		SubjectTokenSupplier: &staticSubjectTokenSupplier{token: originalToken.AccessToken},
		Scopes:               sts.ExternalAccountScopes,
	})
	if err != nil {
		return nil, err
	}
	return ts.Token()
}
