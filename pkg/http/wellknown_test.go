package http

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
)

const defaultWellknownPayload = `{
	"issuer": "https://localhost",
	"registration_endpoint": "https://localhost/clients-registrations/openid-connect",
	"require_request_uri_registration": true,
	"scopes_supported":["scope-1", "scope-2"]
}`

var wellknownPaths = []string{
	".well-known/oauth-authorization-server",
	".well-known/oauth-protected-resource",
	".well-known/openid-configuration",
}

type WellknownSuite struct {
	BaseHttpSuite
	TestServer        *httptest.Server
	TestServerPayload string
	ReceivedRequest   *http.Request
}

func (s *WellknownSuite) SetupTest() {
	s.BaseHttpSuite.SetupTest()
	s.StaticConfig.ClusterProviderStrategy = api.ClusterProviderKubeConfig
	s.TestServerPayload = defaultWellknownPayload
	s.TestServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.EscapedPath(), "/.well-known/") {
			http.NotFound(w, r)
			return
		}
		s.ReceivedRequest = r.Clone(s.T().Context())
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Custom-Backend-Header", "backend-value")
		_, _ = w.Write([]byte(s.TestServerPayload))
	}))
	s.StaticConfig.AuthorizationURL = s.TestServer.URL
}

func (s *WellknownSuite) TearDownTest() {
	s.BaseHttpSuite.TearDownTest()
	if s.TestServer != nil {
		s.TestServer.Close()
	}
}

func (s *WellknownSuite) TestCorsHeaders() {
	var receivedRequestHeaders http.Header
	s.StaticConfig.RequireOAuth = true
	s.StartServer()

	for _, path := range wellknownPaths {
		s.ReceivedRequest = nil
		req, err := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%s/%s", s.StaticConfig.Port, path), nil)
		s.NoError(err, "Failed to create request")
		// Add various headers to test they are NOT propagated
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("X-Custom-Header", "custom-value")

		resp, err := http.DefaultClient.Do(req)
		s.Require().NoErrorf(err, "Failed to get %s endpoint", path)
		s.Require().NotNil(s.ReceivedRequest, "Backend did not receive any request")
		receivedRequestHeaders = s.ReceivedRequest.Header
		s.T().Cleanup(func() { _ = resp.Body.Close() })

		s.Run("Well-known proxy does not propagate client headers to backend for "+path, func() {
			s.Empty(receivedRequestHeaders.Get("Origin"), "Expected backend request to not have Origin header")
			s.Empty(receivedRequestHeaders.Get("X-Custom-Header"), "Expected backend request to not have X-Custom-Header")
		})

		s.Run("Well-known proxy returns CORS Access-Control-Allow-Origin header for "+path, func() {
			s.Equal("*", resp.Header.Get("Access-Control-Allow-Origin"), "Expected Access-Control-Allow-Origin header to be '*'")
		})

		s.Run("Well-known proxy returns CORS Access-Control-Allow-Methods header for "+path, func() {
			s.Equal("GET, OPTIONS", resp.Header.Get("Access-Control-Allow-Methods"), "Expected Access-Control-Allow-Methods header to be 'GET, OPTIONS'")
		})

		s.Run("Well-known proxy returns CORS Access-Control-Allow-Headers header for "+path, func() {
			s.Equal("Content-Type, Authorization", resp.Header.Get("Access-Control-Allow-Headers"), "Expected Access-Control-Allow-Headers header to be 'Content-Type, Authorization'")
		})

		s.Run("Well-known proxy returns Content-Type header for "+path, func() {
			s.Equal("application/json", resp.Header.Get("Content-Type"), "Expected Content-Type header to be 'application/json'")
		})
	}
}

func (s *WellknownSuite) TestResponseHeaderPropagation() {
	s.StaticConfig.RequireOAuth = true
	s.StartServer()

	for _, path := range wellknownPaths {
		s.Run("Well-known proxy propagates backend headers for "+path, func() {
			req, err := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%s/%s", s.StaticConfig.Port, path), nil)
			s.NoError(err, "Failed to create request")

			resp, err := http.DefaultClient.Do(req)
			s.Require().NoErrorf(err, "Failed to get %s endpoint", path)
			s.T().Cleanup(func() { _ = resp.Body.Close() })

			s.Equal("backend-value", resp.Header.Get("Custom-Backend-Header"), "Expected Custom-Backend-Header to be propagated from backend")
		})
	}
}

func (s *WellknownSuite) TestOptionsPreflightRequest() {
	s.StaticConfig.RequireOAuth = true
	s.StartServer()

	for _, path := range wellknownPaths {
		s.Run("Well-known endpoint responds to OPTIONS preflight for "+path, func() {
			req, err := http.NewRequest("OPTIONS", fmt.Sprintf("http://127.0.0.1:%s/%s", s.StaticConfig.Port, path), nil)
			s.Require().NoError(err, "Failed to create request")
			req.Header.Set("Origin", "https://example.com")
			req.Header.Set("Access-Control-Request-Method", "GET")
			req.Header.Set("Access-Control-Request-Headers", "Authorization")

			resp, err := http.DefaultClient.Do(req)
			s.Require().NoErrorf(err, "Failed to get OPTIONS %s endpoint", path)
			s.T().Cleanup(func() { _ = resp.Body.Close() })

			s.Equal("*", resp.Header.Get("Access-Control-Allow-Origin"), "Expected Access-Control-Allow-Origin header to be '*'")
			s.Equal("GET, OPTIONS", resp.Header.Get("Access-Control-Allow-Methods"), "Expected Access-Control-Allow-Methods header to be 'GET, OPTIONS'")
			s.Equal("Content-Type, Authorization", resp.Header.Get("Access-Control-Allow-Headers"), "Expected Access-Control-Allow-Headers header to be 'Content-Type, Authorization'")
		})
	}
}

func (s *WellknownSuite) TestReverseProxyNoAuthURL() {
	s.Run("with no authorization URL configured", func() {
		s.StaticConfig.AuthorizationURL = ""
		s.StaticConfig.RequireOAuth = true
		s.StartServer()

		for _, path := range wellknownPaths {
			s.Run("returns 404 for "+path, func() {
				resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/%s", s.StaticConfig.Port, path))
				s.Require().NoError(err, "Failed to get endpoint")
				s.T().Cleanup(func() { _ = resp.Body.Close() })
				s.Equal(http.StatusNotFound, resp.StatusCode, "Expected HTTP 404 Not Found")
			})
		}
	})
}

func (s *WellknownSuite) TestReverseProxyInvalidPayload() {
	s.Run("with invalid JSON payload from authorization server", func() {
		s.TestServerPayload = `NOT A JSON PAYLOAD`
		s.StaticConfig.RequireOAuth = true
		s.StartServer()

		for _, path := range wellknownPaths {
			s.Run("returns 500 for "+path, func() {
				resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/%s", s.StaticConfig.Port, path))
				s.Require().NoError(err, "Failed to get endpoint")
				s.T().Cleanup(func() { _ = resp.Body.Close() })
				s.Equal(http.StatusInternalServerError, resp.StatusCode, "Expected HTTP 500 Internal Server Error")
			})
		}
	})
}

func (s *WellknownSuite) TestReverseProxyValidPayload() {
	s.Run("with valid payload from authorization server", func() {
		s.StaticConfig.RequireOAuth = true
		s.StartServer()

		for _, path := range wellknownPaths {
			resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/%s", s.StaticConfig.Port, path))
			s.Require().NoError(err, "Failed to get endpoint")
			s.T().Cleanup(func() { _ = resp.Body.Close() })

			s.Run("returns 200 for "+path, func() {
				s.Equal(http.StatusOK, resp.StatusCode, "Expected HTTP 200 OK")
			})
			s.Run("returns application/json content-type for "+path, func() {
				s.Equal("application/json", resp.Header.Get("Content-Type"), "Expected Content-Type application/json")
			})
		}
	})
}

func (s *WellknownSuite) TestDisableDynamicClientRegistration() {
	s.Run("with dynamic client registration disabled", func() {
		s.StaticConfig.RequireOAuth = true
		s.StaticConfig.DisableDynamicClientRegistration = true
		s.StartServer()

		for _, path := range wellknownPaths {
			resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/%s", s.StaticConfig.Port, path))
			s.Require().NoError(err, "Failed to get endpoint")
			body, err := io.ReadAll(resp.Body)
			s.Require().NoError(err, "Failed to read response body")
			s.T().Cleanup(func() { _ = resp.Body.Close() })

			s.Run("removes registration_endpoint for "+path, func() {
				s.NotContains(string(body), "registration_endpoint", "Expected registration_endpoint to be removed")
			})
			s.Run("sets require_request_uri_registration=false for "+path, func() {
				s.Contains(string(body), `"require_request_uri_registration":false`, "Expected require_request_uri_registration to be false")
			})
			s.Run("preserves scopes_supported for "+path, func() {
				s.Contains(string(body), `"scopes_supported":["scope-1","scope-2"]`, "Expected scopes_supported to be preserved")
			})
		}
	})
}

func (s *WellknownSuite) TestOAuthScopesOverride() {
	s.Run("with OAuth scopes override configured", func() {
		s.StaticConfig.RequireOAuth = true
		s.StaticConfig.OAuthScopes = []string{"openid", "mcp-server"}
		s.StartServer()

		for _, path := range wellknownPaths {
			resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/%s", s.StaticConfig.Port, path))
			s.Require().NoError(err, "Failed to get endpoint")
			body, err := io.ReadAll(resp.Body)
			s.Require().NoError(err, "Failed to read response body")
			s.T().Cleanup(func() { _ = resp.Body.Close() })

			s.Run("overrides scopes_supported for "+path, func() {
				s.Contains(string(body), `"scopes_supported":["openid","mcp-server"]`, "Expected scopes_supported to be overridden")
			})
			s.Run("preserves issuer for "+path, func() {
				s.Contains(string(body), `"issuer":"https://localhost"`, "Expected issuer to be preserved")
			})
			s.Run("preserves registration_endpoint for "+path, func() {
				s.Contains(string(body), `"registration_endpoint":"https://localhost`, "Expected registration_endpoint to be preserved")
			})
			s.Run("preserves require_request_uri_registration for "+path, func() {
				s.Contains(string(body), `"require_request_uri_registration":true`, "Expected require_request_uri_registration to be preserved")
			})
		}
	})
}

func TestWellknown(t *testing.T) {
	suite.Run(t, new(WellknownSuite))
}
