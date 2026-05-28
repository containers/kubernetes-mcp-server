package tokenexchange

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"
)

type RFC8693ExchangerTestSuite struct {
	suite.Suite
}

// newRecordingServer returns an httptest.Server that decodes the form body of the
// inbound request into the supplied url.Values pointer and replies with a valid
// token exchange response.
func (s *RFC8693ExchangerTestSuite) newRecordingServer(captured *map[string]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal(http.MethodPost, r.Method)
		s.Equal(ContentTypeXWWWFormUrlEncoded, r.Header.Get(HeaderContentType))

		s.Require().NoError(r.ParseForm())
		out := make(map[string]string, len(r.Form))
		for k := range r.Form {
			out[k] = r.Form.Get(k)
		}
		*captured = out

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "exchanged-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
}

func (s *RFC8693ExchangerTestSuite) TestExchange() {
	s.Run("defaults subject_token_type to access_token when empty", func() {
		var form map[string]string
		server := s.newRecordingServer(&form)
		defer server.Close()

		exchanger := &rfc8693Exchanger{}
		cfg := &TargetTokenExchangeConfig{
			TokenURL: server.URL,
			Audience: "kubernetes-api",
			// SubjectTokenType intentionally empty
		}

		token, err := exchanger.Exchange(context.Background(), cfg, "incoming-token")
		s.Require().NoError(err)
		s.Equal("exchanged-token", token.AccessToken)
		s.Equal(TokenTypeAccessToken, form[FormKeySubjectTokenType],
			"empty SubjectTokenType should default to access_token (RFC 8693 §2.1)")
	})

	s.Run("defaults requested_token_type to access_token when empty", func() {
		var form map[string]string
		server := s.newRecordingServer(&form)
		defer server.Close()

		exchanger := &rfc8693Exchanger{}
		cfg := &TargetTokenExchangeConfig{
			TokenURL: server.URL,
			Audience: "kubernetes-api",
			// RequestedTokenType intentionally empty
		}

		_, err := exchanger.Exchange(context.Background(), cfg, "incoming-token")
		s.Require().NoError(err)
		s.Equal(TokenTypeAccessToken, form[FormKeyRequestedTokenType],
			"empty RequestedTokenType should default to access_token (RFC 8693 §2.1)")
	})

	s.Run("overrides subject_token_type with configured value", func() {
		var form map[string]string
		server := s.newRecordingServer(&form)
		defer server.Close()

		exchanger := &rfc8693Exchanger{}
		cfg := &TargetTokenExchangeConfig{
			TokenURL:         server.URL,
			Audience:         "kubernetes-api",
			SubjectTokenType: TokenTypeJWT,
		}

		_, err := exchanger.Exchange(context.Background(), cfg, "incoming-token")
		s.Require().NoError(err)
		s.Equal(TokenTypeJWT, form[FormKeySubjectTokenType],
			"configured SubjectTokenType should be sent verbatim")
	})

	s.Run("overrides requested_token_type with configured value", func() {
		var form map[string]string
		server := s.newRecordingServer(&form)
		defer server.Close()

		exchanger := &rfc8693Exchanger{}
		cfg := &TargetTokenExchangeConfig{
			TokenURL:           server.URL,
			Audience:           "kubernetes-api",
			RequestedTokenType: TokenTypeJWT,
		}

		_, err := exchanger.Exchange(context.Background(), cfg, "incoming-token")
		s.Require().NoError(err)
		s.Equal(TokenTypeJWT, form[FormKeyRequestedTokenType],
			"configured RequestedTokenType should be sent verbatim")
	})

	s.Run("sends both overrides independently", func() {
		var form map[string]string
		server := s.newRecordingServer(&form)
		defer server.Close()

		exchanger := &rfc8693Exchanger{}
		cfg := &TargetTokenExchangeConfig{
			TokenURL:           server.URL,
			Audience:           "kubernetes-api",
			SubjectTokenType:   TokenTypeJWT,
			RequestedTokenType: TokenTypeJWT,
		}

		_, err := exchanger.Exchange(context.Background(), cfg, "incoming-token")
		s.Require().NoError(err)
		s.Equal(TokenTypeJWT, form[FormKeySubjectTokenType])
		s.Equal(TokenTypeJWT, form[FormKeyRequestedTokenType])
	})

	s.Run("sends mandatory form fields", func() {
		var form map[string]string
		server := s.newRecordingServer(&form)
		defer server.Close()

		exchanger := &rfc8693Exchanger{}
		cfg := &TargetTokenExchangeConfig{
			TokenURL: server.URL,
			Audience: "kubernetes-api",
			Scopes:   []string{"openid", "profile"},
		}

		_, err := exchanger.Exchange(context.Background(), cfg, "incoming-token")
		s.Require().NoError(err)
		s.Equal(GrantTypeTokenExchange, form[FormKeyGrantType])
		s.Equal("incoming-token", form[FormKeySubjectToken])
		s.Equal("kubernetes-api", form[FormKeyAudience])
		s.Equal("openid profile", form[FormKeyScope])
	})

	s.Run("returns error on failed exchange", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid_token"}`))
		}))
		defer server.Close()

		exchanger := &rfc8693Exchanger{}
		cfg := &TargetTokenExchangeConfig{
			TokenURL: server.URL,
			Audience: "kubernetes-api",
		}

		token, err := exchanger.Exchange(context.Background(), cfg, "bad-token")
		s.Require().Error(err)
		s.Nil(token)
		s.Contains(err.Error(), "401")
	})
}

func TestRFC8693Exchanger(t *testing.T) {
	suite.Run(t, new(RFC8693ExchangerTestSuite))
}
