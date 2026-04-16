package tokenexchange

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"
)

type EntraOBOExchangerTestSuite struct {
	suite.Suite
}

func (s *EntraOBOExchangerTestSuite) TestExchange() {
	s.Run("successful token exchange", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.Equal(http.MethodPost, r.Method)
			s.Equal("application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

			err := r.ParseForm()
			s.Require().NoError(err)

			s.Equal(GrantTypeJWTBearer, r.Form.Get(FormKeyGrantType))
			s.Equal("incoming-token", r.Form.Get(FormKeyAssertion))
			s.Equal(RequestedTokenUseOBO, r.Form.Get(FormKeyRequestedUse))
			s.Equal("test-client", r.Form.Get(FormKeyClientID))
			s.Equal("test-secret", r.Form.Get(FormKeyClientSecret))
			s.Equal("api://target/.default", r.Form.Get(FormKeyScope))

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "exchanged-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		}))
		defer server.Close()

		exchanger := &entraOBOExchanger{}
		cfg := &TargetTokenExchangeConfig{
			TokenURL:     server.URL,
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			Scopes:       []string{"api://target/.default"},
		}

		token, err := exchanger.Exchange(context.Background(), cfg, "incoming-token")
		s.Require().NoError(err)
		s.Equal("exchanged-token", token.AccessToken)
		s.Equal("Bearer", token.TokenType)
	})

	s.Run("uses audience as scope when scopes empty", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = r.ParseForm()
			s.Equal("api://kubernetes/.default", r.Form.Get(FormKeyScope))

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "token",
				"token_type":   "Bearer",
			})
		}))
		defer server.Close()

		exchanger := &entraOBOExchanger{}
		cfg := &TargetTokenExchangeConfig{
			TokenURL: server.URL,
			Audience: "api://kubernetes/.default",
		}

		token, err := exchanger.Exchange(context.Background(), cfg, "incoming-token")
		s.Require().NoError(err)
		s.NotEmpty(token.AccessToken)
	})

	s.Run("returns error on failed exchange", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error": "invalid_grant"}`))
		}))
		defer server.Close()

		exchanger := &entraOBOExchanger{}
		cfg := &TargetTokenExchangeConfig{
			TokenURL: server.URL,
		}

		token, err := exchanger.Exchange(context.Background(), cfg, "bad-token")
		s.Error(err)
		s.Nil(token)
		s.Contains(err.Error(), "400")
	})
}

func TestEntraOBOExchanger(t *testing.T) {
	suite.Run(t, new(EntraOBOExchangerTestSuite))
}
