package tokenexchange

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"
)

type KeycloakV2ExchangerTestSuite struct {
	suite.Suite
}

func (s *KeycloakV2ExchangerTestSuite) newRecordingServer(captured *recordedRequest) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.method = r.Method
		captured.contentType = r.Header.Get(HeaderContentType)

		if err := r.ParseForm(); err != nil {
			captured.parseErr = err
		} else {
			out := make(map[string]string, len(r.Form))
			for k := range r.Form {
				out[k] = r.Form.Get(k)
			}
			captured.form = out
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "exchanged-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
}

func (s *KeycloakV2ExchangerTestSuite) TestExchange() {
	s.Run("always sends subject_token_type as access_token", func() {
		var captured recordedRequest
		server := s.newRecordingServer(&captured)
		defer server.Close()

		exchanger := &keycloakV2Exchanger{}
		cfg := &TargetTokenExchangeConfig{
			TokenURL:         server.URL,
			Audience:         "spoke-cluster",
			SubjectTokenType: TokenTypeJWT,
		}

		token, err := exchanger.Exchange(context.Background(), cfg, "incoming-token")
		s.Require().NoError(err)
		s.Require().NoError(captured.parseErr)
		s.Equal("exchanged-token", token.AccessToken)
		s.Equal(TokenTypeAccessToken, captured.form[FormKeySubjectTokenType],
			"v2 must always send access_token as subject_token_type, ignoring config")
	})

	s.Run("never sends subject_issuer", func() {
		var captured recordedRequest
		server := s.newRecordingServer(&captured)
		defer server.Close()

		exchanger := &keycloakV2Exchanger{}
		cfg := &TargetTokenExchangeConfig{
			TokenURL:      server.URL,
			Audience:      "spoke-cluster",
			SubjectIssuer: "external-idp",
		}

		_, err := exchanger.Exchange(context.Background(), cfg, "incoming-token")
		s.Require().NoError(err)
		s.Require().NoError(captured.parseErr)
		_, hasSubjectIssuer := captured.form[FormKeySubjectIssuer]
		s.False(hasSubjectIssuer, "v2 must never send subject_issuer (same-realm only)")
	})

	s.Run("omits requested_token_type when not configured", func() {
		var captured recordedRequest
		server := s.newRecordingServer(&captured)
		defer server.Close()

		exchanger := &keycloakV2Exchanger{}
		cfg := &TargetTokenExchangeConfig{
			TokenURL: server.URL,
			Audience: "spoke-cluster",
		}

		_, err := exchanger.Exchange(context.Background(), cfg, "incoming-token")
		s.Require().NoError(err)
		s.Require().NoError(captured.parseErr)
		_, hasRequestedType := captured.form[FormKeyRequestedTokenType]
		s.False(hasRequestedType,
			"v2 should omit requested_token_type when not configured (Keycloak defaults to access_token)")
	})

	s.Run("sends requested_token_type when configured", func() {
		var captured recordedRequest
		server := s.newRecordingServer(&captured)
		defer server.Close()

		exchanger := &keycloakV2Exchanger{}
		cfg := &TargetTokenExchangeConfig{
			TokenURL:           server.URL,
			Audience:           "spoke-cluster",
			RequestedTokenType: TokenTypeJWT,
		}

		_, err := exchanger.Exchange(context.Background(), cfg, "incoming-token")
		s.Require().NoError(err)
		s.Require().NoError(captured.parseErr)
		s.Equal(TokenTypeJWT, captured.form[FormKeyRequestedTokenType])
	})

	s.Run("sends mandatory form fields", func() {
		var captured recordedRequest
		server := s.newRecordingServer(&captured)
		defer server.Close()

		exchanger := &keycloakV2Exchanger{}
		cfg := &TargetTokenExchangeConfig{
			TokenURL: server.URL,
			Audience: "spoke-cluster",
			Scopes:   []string{"openid", "mcp:spoke"},
		}

		_, err := exchanger.Exchange(context.Background(), cfg, "incoming-token")
		s.Require().NoError(err)
		s.Require().NoError(captured.parseErr)
		s.Equal(http.MethodPost, captured.method)
		s.Equal(ContentTypeXWWWFormUrlEncoded, captured.contentType)
		s.Equal(GrantTypeTokenExchange, captured.form[FormKeyGrantType])
		s.Equal("incoming-token", captured.form[FormKeySubjectToken])
		s.Equal("spoke-cluster", captured.form[FormKeyAudience])
		s.Equal("openid mcp:spoke", captured.form[FormKeyScope])
	})

	s.Run("returns error on failed exchange", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid_token"}`))
		}))
		defer server.Close()

		exchanger := &keycloakV2Exchanger{}
		cfg := &TargetTokenExchangeConfig{
			TokenURL: server.URL,
			Audience: "spoke-cluster",
		}

		token, err := exchanger.Exchange(context.Background(), cfg, "bad-token")
		s.Require().Error(err)
		s.Nil(token)
		s.Contains(err.Error(), "401")
	})
}

func TestKeycloakV2Exchanger(t *testing.T) {
	suite.Run(t, new(KeycloakV2ExchangerTestSuite))
}
