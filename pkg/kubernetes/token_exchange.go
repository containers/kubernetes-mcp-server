package kubernetes

import (
	"context"
	"net/http"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/tokenexchange"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"k8s.io/klog/v2"
)

func ExchangeTokenInContext(
	ctx context.Context,
	stsConfigProvider api.StsConfigProvider,
	oidcProvider *oidc.Provider,
	httpClient *http.Client,
	provider Provider,
	target string,
) context.Context {
	auth, ok := ctx.Value(OAuthAuthorizationHeader).(string)
	if !ok || !strings.HasPrefix(auth, "Bearer ") {
		return ctx
	}
	subjectToken := strings.TrimPrefix(auth, "Bearer ")

	tep, ok := provider.(TokenExchangeProvider)
	if !ok {
		return stsExchangeTokenInContext(ctx, stsConfigProvider, oidcProvider, httpClient, subjectToken)
	}

	exCfg := tep.GetTokenExchangeConfig(target)
	if exCfg == nil {
		return stsExchangeTokenInContext(ctx, stsConfigProvider, oidcProvider, httpClient, subjectToken)
	}

	exchanger, ok := tokenexchange.GetTokenExchanger(tep.GetTokenExchangeStrategy())
	if !ok {
		klog.Warningf("token exchange strategy %q not found in registry", tep.GetTokenExchangeStrategy())
		return stsExchangeTokenInContext(ctx, stsConfigProvider, oidcProvider, httpClient, subjectToken)
	}

	exchanged, err := exchanger.Exchange(ctx, exCfg, subjectToken)
	if err != nil {
		klog.Errorf("token exchange failed for target %q: %v", target, err)
		return ctx
	}

	klog.V(4).Infof("token exchanged successfully for target %q", target)
	return context.WithValue(ctx, OAuthAuthorizationHeader, "Bearer "+exchanged.AccessToken)
}

// TODO(Cali0707): remove this method and move to using the rfc8693 token exchanger for the global token exchange
func stsExchangeTokenInContext(
	ctx context.Context,
	stsConfigProvider api.StsConfigProvider,
	oidcProvider *oidc.Provider,
	httpClient *http.Client,
	token string,
) context.Context {
	sts := NewFromConfig(stsConfigProvider, oidcProvider)
	if !sts.IsEnabled() {
		return ctx
	}

	if httpClient != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	}

	exchangedToken, err := sts.ExternalAccountTokenExchange(ctx, &oauth2.Token{
		AccessToken: token,
		TokenType:   "Bearer",
	})
	if err != nil {
		klog.Errorf("token exchange failed: %v", err)
		return ctx
	}

	klog.V(4).Info("token exchanged successfully")
	return context.WithValue(ctx, OAuthAuthorizationHeader, "Bearer "+exchangedToken.AccessToken)
}
