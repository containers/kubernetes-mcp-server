package tokenexchange

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

const (
	GrantTypeTokenExchange = "urn:ietf:params:oauth:grant-type:token-exchange"
	TokenTypeAccessToken   = "urn:ietf:params:oauth:token-type:access_token"
	TokenTypeJWT           = "urn:ietf:params:oauth:token-type:jwt"
)

const (
	FormKeyGrantType          = "grant_type"
	FormKeySubjectToken       = "subject_token"
	FormKeySubjectTokenType   = "subject_token_type"
	FormKeySubjectIssuer      = "subject_issuer"
	FormKeyAudience           = "audience"
	FormKeyClientID           = "client_id"
	FormKeyClientSecret       = "client_secret"
	FormKeyScope              = "scope"
	FormKeyRequestedTokenType = "requested_token_type"
)

const (
	HeaderContentType             = "Content-Type"
	HeaderAuthorization           = "Authorization"
	ContentTypeXWWWFormUrlEncoded = "application/x-www-form-urlencoded"
)

const (
	StrategyKeycloakV1 = "keycloak-v1"
	StrategyRFC8693    = "rfc8693"
)

type TokenExchanger interface {
	Exchange(ctx context.Context, cfg *TargetTokenExchangeConfig, subjectToken string) (*oauth2.Token, error)
}

// injectClientAuth adds client credentials to the request based on auth style
func injectClientAuth(cfg *TargetTokenExchangeConfig, data url.Values, header http.Header) {
	if cfg.ClientID == "" {
		return
	}

	switch cfg.AuthStyle {
	case AuthStyleHeader:
		credentials := cfg.ClientID + ":" + cfg.ClientSecret
		header.Set(HeaderAuthorization, "Basic "+base64.StdEncoding.EncodeToString([]byte(credentials)))
	default: // AuthStyleParams or empty (default)
		data.Set(FormKeyClientID, cfg.ClientID)
		if cfg.ClientSecret != "" {
			data.Set(FormKeyClientSecret, cfg.ClientSecret)
		}
	}
}

// tokenExchangeResponse represents the OAuth token exchange response
type tokenExchangeResponse struct {
	AccessToken     string `json:"access_token"`
	TokenType       string `json:"token_type"`
	ExpiresIn       int    `json:"expires_in"`
	RefreshToken    string `json:"refresh_token,omitempty"`
	Scope           string `json:"scope,omitempty"`
	IssuedTokenType string `json:"issued_token_type,omitempty"`
}

func doTokenExchange(ctx context.Context, httpClient *http.Client, tokenURL string, data url.Values, headers http.Header) (*oauth2.Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token exchange request: %w", err)
	}

	req.Header.Set(HeaderContentType, ContentTypeXWWWFormUrlEncoded)
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token exchange response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	tokenResp := &tokenExchangeResponse{}
	if err := json.Unmarshal(body, tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token exchange response: %w", err)
	}

	token := &oauth2.Token{
		AccessToken:  tokenResp.AccessToken,
		TokenType:    tokenResp.TokenType,
		RefreshToken: tokenResp.RefreshToken,
	}

	if tokenResp.ExpiresIn > 0 {
		token.Expiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}

	return token, nil
}
