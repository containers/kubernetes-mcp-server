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

// recordedRequest captures what the handler observed. Assertions happen in the
// test goroutine after Exchange returns — require.NoError/FailNow from inside
// the httptest handler goroutine only unwinds that goroutine, which can hang
// or mask the real failure instead of failing the test.
type recordedRequest struct {
	method      string
	contentType string
	parseErr    error
	form        map[string]string
}

// newRecordingServer returns an httptest.Server that decodes the form body of the
// inbound request into the supplied recordedRequest and replies with a valid
// token exchange response.
func (s *RFC8693ExchangerTestSuite) newRecordingServer(captured *recordedRequest) *httptest.Server {
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

func (s *RFC8693ExchangerTestSuite) TestExchange() {
	s.Run("defaults subject_token_type to access_token when empty", func() {
		var captured recordedRequest
		server := s.newRecordingServer(&captured)
		defer server.Close()

		exchanger := &rfc8693Exchanger{}
		cfg := &TargetTokenExchangeConfig{
			TokenURL: server.URL,
			Audience: "kubernetes-api",
			// SubjectTokenType intentionally empty
		}

		token, err := exchanger.Exchange(context.Background(), cfg, "incoming-token")
		s.Require().NoError(err)
		s.Require().NoError(captured.parseErr)
		s.Equal(http.MethodPost, captured.method)
		s.Equal(ContentTypeXWWWFormUrlEncoded, captured.contentType)
		s.Equal("exchanged-token", token.AccessToken)
		s.Equal(TokenTypeAccessToken, captured.form[FormKeySubjectTokenType],
			"empty SubjectTokenType should default to access_token (RFC 8693 section 2.1)")
	})

	s.Run("defaults requested_token_type to access_token when empty", func() {
		var captured recordedRequest
		server := s.newRecordingServer(&captured)
		defer server.Close()

		exchanger := &rfc8693Exchanger{}
		cfg := &TargetTokenExchangeConfig{
			TokenURL: server.URL,
			Audience: "kubernetes-api",
			// RequestedTokenType intentionally empty
		}

		_, err := exchanger.Exchange(context.Background(), cfg, "incoming-token")
		s.Require().NoError(err)
		s.Require().NoError(captured.parseErr)
		s.Equal(TokenTypeAccessToken, captured.form[FormKeyRequestedTokenType],
			"empty RequestedTokenType should default to access_token (RFC 8693 section 2.1)")
	})

	s.Run("overrides subject_token_type with configured value", func() {
		var captured recordedRequest
		server := s.newRecordingServer(&captured)
		defer server.Close()

		exchanger := &rfc8693Exchanger{}
		cfg := &TargetTokenExchangeConfig{
			TokenURL:         server.URL,
			Audience:         "kubernetes-api",
			SubjectTokenType: TokenTypeJWT,
		}

		_, err := exchanger.Exchange(context.Background(), cfg, "incoming-token")
		s.Require().NoError(err)
		s.Require().NoError(captured.parseErr)
		s.Equal(TokenTypeJWT, captured.form[FormKeySubjectTokenType],
			"configured SubjectTokenType should be sent verbatim")
	})

	s.Run("overrides requested_token_type with configured value", func() {
		var captured recordedRequest
		server := s.newRecordingServer(&captured)
		defer server.Close()

		exchanger := &rfc8693Exchanger{}
		cfg := &TargetTokenExchangeConfig{
			TokenURL:           server.URL,
			Audience:           "kubernetes-api",
			RequestedTokenType: TokenTypeJWT,
		}

		_, err := exchanger.Exchange(context.Background(), cfg, "incoming-token")
		s.Require().NoError(err)
		s.Require().NoError(captured.parseErr)
		s.Equal(TokenTypeJWT, captured.form[FormKeyRequestedTokenType],
			"configured RequestedTokenType should be sent verbatim")
	})

	s.Run("sends both overrides independently", func() {
		var captured recordedRequest
		server := s.newRecordingServer(&captured)
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
		s.Require().NoError(captured.parseErr)
		s.Equal(TokenTypeJWT, captured.form[FormKeySubjectTokenType])
		s.Equal(TokenTypeJWT, captured.form[FormKeyRequestedTokenType])
	})

	s.Run("sends mandatory form fields", func() {
		var captured recordedRequest
		server := s.newRecordingServer(&captured)
		defer server.Close()

		exchanger := &rfc8693Exchanger{}
		cfg := &TargetTokenExchangeConfig{
			TokenURL: server.URL,
			Audience: "kubernetes-api",
			Scopes:   []string{"openid", "profile"},
		}

		_, err := exchanger.Exchange(context.Background(), cfg, "incoming-token")
		s.Require().NoError(err)
		s.Require().NoError(captured.parseErr)
		s.Equal(GrantTypeTokenExchange, captured.form[FormKeyGrantType])
		s.Equal("incoming-token", captured.form[FormKeySubjectToken])
		s.Equal("kubernetes-api", captured.form[FormKeyAudience])
		s.Equal("openid profile", captured.form[FormKeyScope])
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
