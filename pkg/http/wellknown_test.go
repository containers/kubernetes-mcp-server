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
		if !strings.Contains(r.URL.EscapedPath(), "/.well-known/") {
			http.NotFound(w, r)
			return
		}
		s.ReceivedRequest = r.Clone(s.T().Context())
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Custom-Backend-Header", "backend-value")
		w.Header().Set("Server", "TestIdP/1.0")
		w.Header().Set("Set-Cookie", "session=abc123")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
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

func (s *WellknownSuite) TestResponseHeaderFiltering() {
	s.StaticConfig.RequireOAuth = true
	s.StartServer()

	for _, path := range wellknownPaths {
		s.Run("propagates allowed headers for "+path, func() {
			resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/%s", s.StaticConfig.Port, path))
			s.Require().NoError(err)
			s.T().Cleanup(func() { _ = resp.Body.Close() })
			s.Equal("no-cache", resp.Header.Get("Cache-Control"))
		})
		s.Run("does not propagate non-allowed headers for "+path, func() {
			resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/%s", s.StaticConfig.Port, path))
			s.Require().NoError(err)
			s.T().Cleanup(func() { _ = resp.Body.Close() })
			s.Empty(resp.Header.Get("Custom-Backend-Header"), "Custom backend headers should not be forwarded")
			s.Empty(resp.Header.Get("Server"), "Server header should not be forwarded")
			s.Empty(resp.Header.Get("Set-Cookie"), "Set-Cookie header should not be forwarded")
			s.Empty(resp.Header.Get("Strict-Transport-Security"), "HSTS header should not be forwarded")
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

func (s *WellknownSuite) TestPathTraversal() {
	s.StaticConfig.RequireOAuth = true
	s.StartServer()

	traversalPaths := []string{
		".well-known/oauth-authorization-server/../../admin",
		".well-known/oauth-authorization-server/../../../etc/passwd",
		".well-known/../secrets",
	}
	for _, path := range traversalPaths {
		s.Run("rejects path traversal for "+path, func() {
			resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/%s", s.StaticConfig.Port, path))
			s.Require().NoError(err)
			s.T().Cleanup(func() { _ = resp.Body.Close() })
			s.NotEqual(http.StatusOK, resp.StatusCode, "Path traversal request should not succeed")
		})
	}
}

func (s *WellknownSuite) TestUpstreamPathStaysWithinWellKnown() {
	s.StaticConfig.RequireOAuth = true
	s.StartServer()

	for _, path := range wellknownPaths {
		s.Run("upstream request path starts with /.well-known/ for "+path, func() {
			s.ReceivedRequest = nil
			resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/%s", s.StaticConfig.Port, path))
			s.Require().NoError(err)
			s.T().Cleanup(func() { _ = resp.Body.Close() })
			s.Require().NotNil(s.ReceivedRequest)
			s.True(
				strings.HasPrefix(s.ReceivedRequest.URL.Path, "/.well-known/"),
				"Upstream request path %q should start with /.well-known/", s.ReceivedRequest.URL.Path,
			)
		})
	}
}

func (s *WellknownSuite) TestAuthorizationURLWithBasePath() {
	s.StaticConfig.RequireOAuth = true
	s.StaticConfig.AuthorizationURL = s.TestServer.URL + "/realms/openshift"
	s.StartServer()

	for _, path := range wellknownPaths {
		s.Run("proxies correctly with base path for "+path, func() {
			s.ReceivedRequest = nil
			resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/%s", s.StaticConfig.Port, path))
			s.Require().NoError(err)
			s.T().Cleanup(func() { _ = resp.Body.Close() })
			s.Require().NotNil(s.ReceivedRequest)
			s.True(
				strings.HasPrefix(s.ReceivedRequest.URL.Path, "/realms/openshift/.well-known/"),
				"Upstream request path %q should start with /realms/openshift/.well-known/", s.ReceivedRequest.URL.Path,
			)
		})
	}
}

func (s *WellknownSuite) TestOversizedUpstreamResponse() {
	s.Run("rejects upstream response exceeding size limit", func() {
		s.TestServerPayload = `{"data":"` + strings.Repeat("x", 2*1024*1024) + `"}`
		s.StaticConfig.RequireOAuth = true
		s.StartServer()

		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/%s", s.StaticConfig.Port, wellknownPaths[0]))
		s.Require().NoError(err)
		s.T().Cleanup(func() { _ = resp.Body.Close() })
		s.Equal(http.StatusInternalServerError, resp.StatusCode, "Oversized response should be rejected")
	})
}

func (s *WellknownSuite) TestMetadataGenerationFallback() {
	s.Run("generates oauth-authorization-server from openid-configuration when endpoint returns 404", func() {
		// Simulate OIDC providers that only implement openid-configuration (e.g., Entra ID, Auth0)
		s.TestServer.Close()
		s.TestServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.EscapedPath() {
			case "/.well-known/openid-configuration":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"issuer": "https://login.microsoftonline.com/tenant/v2.0",
					"authorization_endpoint": "https://login.microsoftonline.com/tenant/oauth2/v2.0/authorize",
					"token_endpoint": "https://login.microsoftonline.com/tenant/oauth2/v2.0/token",
					"jwks_uri": "https://login.microsoftonline.com/tenant/discovery/v2.0/keys",
					"scopes_supported": ["openid", "profile", "email"]
				}`))
			default:
				http.NotFound(w, r)
			}
		}))
		s.StaticConfig.AuthorizationURL = s.TestServer.URL
		s.StaticConfig.RequireOAuth = true
		s.StartServer()

		// oauth-authorization-server should work via fallback
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/.well-known/oauth-authorization-server", s.StaticConfig.Port))
		s.Require().NoError(err)
		s.T().Cleanup(func() { _ = resp.Body.Close() })

		s.Equal(http.StatusOK, resp.StatusCode, "Expected fallback to succeed")

		body, err := io.ReadAll(resp.Body)
		s.Require().NoError(err)
		s.Contains(string(body), "login.microsoftonline.com", "Expected Entra ID issuer in response")
		s.Contains(string(body), "authorization_endpoint", "Expected authorization_endpoint in response")
	})

	s.Run("generates RFC 9728 compliant oauth-protected-resource when endpoint returns 404", func() {
		s.TestServer.Close()
		s.TestServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.EscapedPath() {
			case "/.well-known/openid-configuration":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"issuer": "https://login.microsoftonline.com/tenant/v2.0",
					"token_endpoint": "https://login.microsoftonline.com/tenant/oauth2/v2.0/token",
					"scopes_supported": ["openid", "profile"]
				}`))
			default:
				http.NotFound(w, r)
			}
		}))
		s.StaticConfig.AuthorizationURL = s.TestServer.URL
		s.StaticConfig.RequireOAuth = true
		s.StartServer()

		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/.well-known/oauth-protected-resource", s.StaticConfig.Port))
		s.Require().NoError(err)
		s.T().Cleanup(func() { _ = resp.Body.Close() })

		s.Equal(http.StatusOK, resp.StatusCode, "Expected fallback to succeed")

		body, err := io.ReadAll(resp.Body)
		s.Require().NoError(err)

		// Verify RFC 9728 format - MCP server is the authorization_server from client's perspective
		s.Contains(string(body), `"authorization_servers":`, "Expected authorization_servers array per RFC 9728")
		s.Contains(string(body), fmt.Sprintf("127.0.0.1:%s", s.StaticConfig.Port), "Expected authorization_servers to contain MCP server URL")
		s.Contains(string(body), `"scopes_supported":`, "Expected scopes_supported from openid-configuration")
	})

	s.Run("falls back to openid-configuration when upstream returns 200 with empty body", func() {
		// Entra ID returns HTTP 200 with content-length: 0 for unsupported well-known paths
		s.TestServer.Close()
		s.TestServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.EscapedPath() {
			case "/.well-known/openid-configuration":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"issuer": "https://login.microsoftonline.com/tenant/v2.0",
					"authorization_endpoint": "https://login.microsoftonline.com/tenant/oauth2/v2.0/authorize",
					"token_endpoint": "https://login.microsoftonline.com/tenant/oauth2/v2.0/token",
					"scopes_supported": ["openid", "profile", "email"]
				}`))
			default:
				w.Header().Set("Content-Length", "0")
				w.WriteHeader(http.StatusOK)
			}
		}))
		s.StaticConfig.AuthorizationURL = s.TestServer.URL
		s.StaticConfig.RequireOAuth = true
		s.StartServer()

		s.Run("oauth-authorization-server", func() {
			resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/.well-known/oauth-authorization-server", s.StaticConfig.Port))
			s.Require().NoError(err)
			s.T().Cleanup(func() { _ = resp.Body.Close() })

			s.Equal(http.StatusOK, resp.StatusCode, "Expected fallback to succeed")

			body, err := io.ReadAll(resp.Body)
			s.Require().NoError(err)
			s.Contains(string(body), "login.microsoftonline.com", "Expected Entra ID issuer in response")
		})

		s.Run("oauth-protected-resource", func() {
			resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/.well-known/oauth-protected-resource", s.StaticConfig.Port))
			s.Require().NoError(err)
			s.T().Cleanup(func() { _ = resp.Body.Close() })

			s.Equal(http.StatusOK, resp.StatusCode, "Expected fallback to succeed")

			body, err := io.ReadAll(resp.Body)
			s.Require().NoError(err)
			s.Contains(string(body), `"authorization_servers":`, "Expected authorization_servers in response")
		})
	})

	s.Run("returns 404 when both oauth-authorization-server and openid-configuration return 404", func() {
		s.TestServer.Close()
		s.TestServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))
		s.StaticConfig.AuthorizationURL = s.TestServer.URL
		s.StaticConfig.RequireOAuth = true
		s.StartServer()

		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/.well-known/oauth-authorization-server", s.StaticConfig.Port))
		s.Require().NoError(err)
		s.T().Cleanup(func() { _ = resp.Body.Close() })

		s.Equal(http.StatusNotFound, resp.StatusCode, "Expected 404 when all endpoints fail")
	})

	s.Run("applies config overrides to fallback response", func() {
		s.TestServer.Close()
		s.TestServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.EscapedPath() {
			case "/.well-known/openid-configuration":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"issuer": "https://login.microsoftonline.com/tenant/v2.0",
					"scopes_supported": ["openid"],
					"registration_endpoint": "https://should-be-removed"
				}`))
			default:
				http.NotFound(w, r)
			}
		}))
		s.StaticConfig.AuthorizationURL = s.TestServer.URL
		s.StaticConfig.RequireOAuth = true
		s.StaticConfig.DisableDynamicClientRegistration = true
		s.StaticConfig.OAuthScopes = []string{"custom-scope"}
		s.StartServer()

		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/.well-known/oauth-authorization-server", s.StaticConfig.Port))
		s.Require().NoError(err)
		s.T().Cleanup(func() { _ = resp.Body.Close() })

		body, err := io.ReadAll(resp.Body)
		s.Require().NoError(err)
		s.NotContains(string(body), "registration_endpoint", "registration_endpoint should be removed")
		s.Contains(string(body), `"scopes_supported":["custom-scope"]`, "scopes should be overridden")
	})
}

func TestWellknown(t *testing.T) {
	suite.Run(t, new(WellknownSuite))
}
