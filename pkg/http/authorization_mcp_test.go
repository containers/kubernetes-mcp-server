package http

import (
	"flag"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"

	"github.com/containers/kubernetes-mcp-server/internal/test"
)

type AuthorizationSuite struct {
	BaseHttpSuite
	mcpClient *test.McpClient
	klogState klog.State
	logBuffer test.SyncBuffer
}

func (s *AuthorizationSuite) SetupTest() {
	s.BaseHttpSuite.SetupTest()

	// Capture logs
	s.logBuffer.Reset()
	s.klogState = klog.CaptureState()
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	klog.InitFlags(flags)
	_ = flags.Set("v", "5")
	klog.SetLogger(textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(5), textlogger.Output(&s.logBuffer))))

	// Default Auth settings (overridden in tests as needed)
	s.OidcProvider = nil
	s.StaticConfig.RequireOAuth = true
	s.StaticConfig.OAuthAudience = ""
	s.StaticConfig.StsClientId = ""
	s.StaticConfig.StsClientSecret = ""
	s.StaticConfig.StsAudience = ""
	s.StaticConfig.StsScopes = []string{}
}

func (s *AuthorizationSuite) TearDownTest() {
	s.BaseHttpSuite.TearDownTest()
	s.klogState.Restore()

	if s.mcpClient != nil {
		s.mcpClient.Close()
		s.mcpClient = nil
	}
}

func (s *AuthorizationSuite) StartClient(headers ...map[string]string) {
	endpoint := fmt.Sprintf("http://127.0.0.1:%s/mcp", s.StaticConfig.Port)
	options := []test.McpClientOption{
		test.WithEndpoint(endpoint),
		test.WithAllowConnectionError(),
	}
	if len(headers) > 0 && len(headers[0]) > 0 {
		options = append(options, test.WithHTTPHeaders(headers[0]))
	}
	s.mcpClient = test.NewMcpClient(s.T(), nil, options...)
}

func (s *AuthorizationSuite) HttpGet(authHeader string) *http.Response {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%s/mcp", s.StaticConfig.Port), nil)
	s.Require().NoError(err, "Failed to create request")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	resp, err := http.DefaultClient.Do(req)
	s.Require().NoError(err, "Failed to get protected endpoint")
	return resp
}

func (s *AuthorizationSuite) TestAuthorizationUnauthorizedMissingHeader() {
	// Missing Authorization header
	s.StartServer()
	s.StartClient()

	s.Run("Initialize returns error for MISSING Authorization header", func() {
		s.Nil(s.mcpClient.Session, "Expected nil session for failed authentication")
	})

	s.Run("Protected resource with MISSING Authorization header", func() {
		resp := s.HttpGet("")
		s.T().Cleanup(func() { _ = resp.Body.Close })

		s.Run("returns 401 - Unauthorized status", func() {
			s.Equal(401, resp.StatusCode, "Expected HTTP 401 for MISSING Authorization header")
		})
		s.Run("returns WWW-Authenticate header", func() {
			authHeader := resp.Header.Get("WWW-Authenticate")
			expected := `Bearer realm="Kubernetes MCP Server", error="missing_token"`
			s.Equal(expected, authHeader, "Expected WWW-Authenticate header to match")
		})
		s.Run("logs error", func() {
			s.Contains(s.logBuffer.String(), "Authentication failed - missing or invalid bearer token", "Expected log entry for missing or invalid bearer token")
		})
	})
}

func (s *AuthorizationSuite) TestAuthorizationUnauthorizedHeaderIncompatible() {
	// Authorization header without Bearer prefix
	s.StartServer()
	s.StartClient(map[string]string{
		"Authorization": "Basic YWxhZGRpbjpvcGVuc2VzYW1l",
	})

	s.Run("Initialize returns error for INCOMPATIBLE Authorization header", func() {
		s.Nil(s.mcpClient.Session, "Expected nil session for failed authentication")
	})

	s.Run("Protected resource with INCOMPATIBLE Authorization header", func() {
		resp := s.HttpGet("Basic YWxhZGRpbjpvcGVuc2VzYW1l")
		s.T().Cleanup(func() { _ = resp.Body.Close })

		s.Run("returns 401 - Unauthorized status", func() {
			s.Equal(401, resp.StatusCode, "Expected HTTP 401 for INCOMPATIBLE Authorization header")
		})
		s.Run("returns WWW-Authenticate header", func() {
			authHeader := resp.Header.Get("WWW-Authenticate")
			expected := `Bearer realm="Kubernetes MCP Server", error="missing_token"`
			s.Equal(expected, authHeader, "Expected WWW-Authenticate header to match")
		})
		s.Run("logs error", func() {
			s.Contains(s.logBuffer.String(), "Authentication failed - missing or invalid bearer token", "Expected log entry for missing or invalid bearer token")
		})
	})
}

func (s *AuthorizationSuite) TestAuthorizationUnauthorizedHeaderInvalid() {
	// Invalid Authorization header
	s.StartServer()
	s.StartClient(map[string]string{
		"Authorization": "Bearer " + strings.ReplaceAll(tokenBasicNotExpired, ".", ".invalid"),
	})

	s.Run("Initialize returns error for INVALID Authorization header", func() {
		s.Nil(s.mcpClient.Session, "Expected nil session for failed authentication")
	})

	s.Run("Protected resource with INVALID Authorization header", func() {
		resp := s.HttpGet("Bearer " + strings.ReplaceAll(tokenBasicNotExpired, ".", ".invalid"))
		s.T().Cleanup(func() { _ = resp.Body.Close })

		s.Run("returns 401 - Unauthorized status", func() {
			s.Equal(401, resp.StatusCode, "Expected HTTP 401 for INVALID Authorization header")
		})
		s.Run("returns WWW-Authenticate header", func() {
			authHeader := resp.Header.Get("WWW-Authenticate")
			expected := `Bearer realm="Kubernetes MCP Server", error="invalid_token"`
			s.Equal(expected, authHeader, "Expected WWW-Authenticate header to match")
		})
		s.Run("logs error", func() {
			s.Contains(s.logBuffer.String(), "Authentication failed - JWT validation error", "Expected log entry for JWT validation error")
			s.Contains(s.logBuffer.String(), "error: failed to parse JWT token: illegal base64 data", "Expected log entry for JWT validation error details")
		})
	})
}

func (s *AuthorizationSuite) TestAuthorizationUnauthorizedHeaderExpired() {
	// Expired Authorization Bearer token
	s.StartServer()
	s.StartClient(map[string]string{
		"Authorization": "Bearer " + tokenBasicExpired,
	})

	s.Run("Initialize returns error for EXPIRED Authorization header", func() {
		s.Nil(s.mcpClient.Session, "Expected nil session for failed authentication")
	})

	s.Run("Protected resource with EXPIRED Authorization header", func() {
		resp := s.HttpGet("Bearer " + tokenBasicExpired)
		s.T().Cleanup(func() { _ = resp.Body.Close })

		s.Run("returns 401 - Unauthorized status", func() {
			s.Equal(401, resp.StatusCode, "Expected HTTP 401 for EXPIRED Authorization header")
		})
		s.Run("returns WWW-Authenticate header", func() {
			authHeader := resp.Header.Get("WWW-Authenticate")
			expected := `Bearer realm="Kubernetes MCP Server", error="invalid_token"`
			s.Equal(expected, authHeader, "Expected WWW-Authenticate header to match")
		})
		s.Run("logs error", func() {
			s.Contains(s.logBuffer.String(), "Authentication failed - JWT validation error", "Expected log entry for JWT validation error")
			s.Contains(s.logBuffer.String(), "validation failed, token is expired (exp)", "Expected log entry for JWT validation error details")
		})
	})
}

func (s *AuthorizationSuite) TestAuthorizationUnauthorizedHeaderInvalidAudience() {
	// Invalid audience claim Bearer token
	s.StaticConfig.OAuthAudience = "expected-audience"
	s.StartServer()
	s.StartClient(map[string]string{
		"Authorization": "Bearer " + tokenBasicNotExpired,
	})

	s.Run("Initialize returns error for INVALID AUDIENCE Authorization header", func() {
		s.Nil(s.mcpClient.Session, "Expected nil session for failed authentication")
	})

	s.Run("Protected resource with INVALID AUDIENCE Authorization header", func() {
		resp := s.HttpGet("Bearer " + tokenBasicNotExpired)
		s.T().Cleanup(func() { _ = resp.Body.Close })

		s.Run("returns 401 - Unauthorized status", func() {
			s.Equal(401, resp.StatusCode, "Expected HTTP 401 for INVALID AUDIENCE Authorization header")
		})
		s.Run("returns WWW-Authenticate header", func() {
			authHeader := resp.Header.Get("WWW-Authenticate")
			expected := `Bearer realm="Kubernetes MCP Server", audience="expected-audience", error="invalid_token"`
			s.Equal(expected, authHeader, "Expected WWW-Authenticate header to match")
		})
		s.Run("logs error", func() {
			s.Contains(s.logBuffer.String(), "Authentication failed - JWT validation error", "Expected log entry for JWT validation error")
			s.Contains(s.logBuffer.String(), "invalid audience claim (aud)", "Expected log entry for JWT validation error details")
		})
	})
}

func (s *AuthorizationSuite) TestAuthorizationUnauthorizedOidcValidation() {
	// Failed OIDC validation
	s.StaticConfig.OAuthAudience = "mcp-server"
	oidcTestServer := NewOidcTestServer(s.T())
	s.T().Cleanup(oidcTestServer.Close)
	s.OidcProvider = oidcTestServer.Provider
	s.StartServer()
	s.StartClient(map[string]string{
		"Authorization": "Bearer " + tokenBasicNotExpired,
	})

	s.Run("Initialize returns error for INVALID OIDC Authorization header", func() {
		s.Nil(s.mcpClient.Session, "Expected nil session for failed authentication")
	})

	s.Run("Protected resource with INVALID OIDC Authorization header", func() {
		resp := s.HttpGet("Bearer " + tokenBasicNotExpired)
		s.T().Cleanup(func() { _ = resp.Body.Close })

		s.Run("returns 401 - Unauthorized status", func() {
			s.Equal(401, resp.StatusCode, "Expected HTTP 401 for INVALID OIDC Authorization header")
		})
		s.Run("returns WWW-Authenticate header", func() {
			authHeader := resp.Header.Get("WWW-Authenticate")
			expected := `Bearer realm="Kubernetes MCP Server", audience="mcp-server", error="invalid_token"`
			s.Equal(expected, authHeader, "Expected WWW-Authenticate header to match")
		})
		s.Run("logs error", func() {
			s.Contains(s.logBuffer.String(), "Authentication failed - JWT validation error", "Expected log entry for JWT validation error")
			s.Contains(s.logBuffer.String(), "OIDC token validation error: failed to verify signature", "Expected log entry for OIDC validation error details")
		})
	})
}

func (s *AuthorizationSuite) TestAuthorizationUnauthorizedTokenExchangeFailure() {
	s.MockServer.ResetHandlers()

	oidcTestServer := NewOidcTestServer(s.T())
	s.T().Cleanup(oidcTestServer.Close)
	rawClaims := `{
		"iss": "` + oidcTestServer.URL + `",
		"exp": ` + strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + `,
		"aud": "%s"
	}`
	validOidcClientToken := oidctest.SignIDToken(oidcTestServer.PrivateKey, "test-oidc-key-id", oidc.RS256,
		fmt.Sprintf(rawClaims, "mcp-server"))
	oidcTestServer.TokenEndpointHandler = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}

	s.OidcProvider = oidcTestServer.Provider
	s.StaticConfig.OAuthAudience = "mcp-server"
	s.StaticConfig.StsClientId = "test-sts-client-id"
	s.StaticConfig.StsClientSecret = "test-sts-client-secret"
	s.StaticConfig.StsAudience = "backend-audience"
	s.StaticConfig.StsScopes = []string{"backend-scope"}
	s.logBuffer.Reset()
	s.StartServer()
	s.StartClient(map[string]string{
		"Authorization": "Bearer " + validOidcClientToken,
	})

	s.Run("Protected resource", func() {
		s.Run("Initialize returns OK for VALID OIDC EXCHANGE Authorization header", func() {
			s.Require().NotNil(s.mcpClient.Session, "Expected session for valid authentication")
			s.Require().NotNil(s.mcpClient.Session.InitializeResult(), "Expected initial request to not be nil")
		})
		s.Run("Call tool exchanges token VALID OIDC EXCHANGE Authorization header", func() {
			toolResult, err := s.mcpClient.Session.CallTool(s.T().Context(), &mcp.CallToolParams{
				Name:      "events_list",
				Arguments: map[string]any{},
			})
			s.Require().NoError(err, "Expected no error calling tool")           // TODO: Should error
			s.Require().NotNil(toolResult, "Expected tool result to not be nil") // Should be nil
			s.Regexp("token exchange failed:[^:]+: status code 401", s.logBuffer.String())
		})
	})
	s.mcpClient.Close()
	s.mcpClient = nil
	s.StopServer()
	s.Require().NoError(s.WaitForShutdown())
}

func (s *AuthorizationSuite) TestAuthorizationRequireOAuthFalse() {
	s.StaticConfig.RequireOAuth = false
	s.StartServer()
	s.StartClient()

	s.Run("Initialize returns OK for MISSING Authorization header", func() {
		s.Require().NotNil(s.mcpClient.Session, "Expected session for successful authentication")
		s.Require().NotNil(s.mcpClient.Session.InitializeResult(), "Expected initial request to not be nil")
	})
	s.mcpClient.Close()
	s.mcpClient = nil
	s.StopServer()
	s.Require().NoError(s.WaitForShutdown())
}

func (s *AuthorizationSuite) TestAuthorizationRawToken() {
	s.MockServer.ResetHandlers()

	cases := []string{"", "mcp-server"}
	for _, audience := range cases {
		s.StaticConfig.OAuthAudience = audience
		s.logBuffer.Reset()
		s.StartServer()
		s.StartClient(map[string]string{
			"Authorization": "Bearer " + tokenBasicNotExpired,
		})

		s.Run(fmt.Sprintf("Protected resource with audience = '%s'", audience), func() {
			s.Run("Initialize returns OK for VALID Authorization header", func() {
				s.Require().NotNil(s.mcpClient.Session, "Expected session for successful authentication")
				s.Require().NotNil(s.mcpClient.Session.InitializeResult(), "Expected initial request to not be nil")
			})
		})
		_ = s.mcpClient.Session.Close()
		s.mcpClient.Session = nil
		s.StopServer()
		s.Require().NoError(s.WaitForShutdown())
	}
}

func (s *AuthorizationSuite) TestAuthorizationOidcToken() {
	s.MockServer.ResetHandlers()

	oidcTestServer := NewOidcTestServer(s.T())
	s.T().Cleanup(oidcTestServer.Close)
	rawClaims := `{
		"iss": "` + oidcTestServer.URL + `",
		"exp": ` + strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + `,
		"aud": "mcp-server"
	}`
	validOidcToken := oidctest.SignIDToken(oidcTestServer.PrivateKey, "test-oidc-key-id", oidc.RS256, rawClaims)

	s.OidcProvider = oidcTestServer.Provider
	s.StaticConfig.OAuthAudience = "mcp-server"
	s.StartServer()
	s.StartClient(map[string]string{
		"Authorization": "Bearer " + validOidcToken,
	})

	s.Run("Protected resource", func() {
		s.Run("Initialize returns OK for VALID OIDC Authorization header", func() {
			s.Require().NotNil(s.mcpClient.Session, "Expected session for successful authentication")
			s.Require().NotNil(s.mcpClient.Session.InitializeResult(), "Expected initial request to not be nil")
		})
	})
	s.mcpClient.Close()
	s.mcpClient = nil
	s.StopServer()
	s.Require().NoError(s.WaitForShutdown())
}

func (s *AuthorizationSuite) TestAuthorizationOidcTokenExchange() {
	s.MockServer.ResetHandlers()

	oidcTestServer := NewOidcTestServer(s.T())
	s.T().Cleanup(oidcTestServer.Close)
	rawClaims := `{
		"iss": "` + oidcTestServer.URL + `",
		"exp": ` + strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + `,
		"aud": "%s"
	}`
	validOidcClientToken := oidctest.SignIDToken(oidcTestServer.PrivateKey, "test-oidc-key-id", oidc.RS256,
		fmt.Sprintf(rawClaims, "mcp-server"))
	validOidcBackendToken := oidctest.SignIDToken(oidcTestServer.PrivateKey, "test-oidc-key-id", oidc.RS256,
		fmt.Sprintf(rawClaims, "backend-audience"))
	oidcTestServer.TokenEndpointHandler = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":"%s","token_type":"Bearer","expires_in":253402297199}`, validOidcBackendToken)
	}

	s.OidcProvider = oidcTestServer.Provider
	s.StaticConfig.OAuthAudience = "mcp-server"
	s.StaticConfig.StsClientId = "test-sts-client-id"
	s.StaticConfig.StsClientSecret = "test-sts-client-secret"
	s.StaticConfig.StsAudience = "backend-audience"
	s.StaticConfig.StsScopes = []string{"backend-scope"}
	s.logBuffer.Reset()
	s.StartServer()
	s.StartClient(map[string]string{
		"Authorization": "Bearer " + validOidcClientToken,
	})

	s.Run("Protected resource", func() {
		s.Run("Initialize returns OK for VALID OIDC EXCHANGE Authorization header", func() {
			s.Require().NotNil(s.mcpClient.Session, "Expected session for successful authentication")
			s.Require().NotNil(s.mcpClient.Session.InitializeResult(), "Expected initial request to not be nil")
		})
		s.Run("Call tool exchanges token VALID OIDC EXCHANGE Authorization header", func() {
			toolResult, err := s.mcpClient.Session.CallTool(s.T().Context(), &mcp.CallToolParams{
				Name:      "events_list",
				Arguments: map[string]any{},
			})
			s.Require().NoError(err, "Expected no error calling tool")
			s.Require().NotNil(toolResult, "Expected tool result to not be nil")
			s.Contains(s.logBuffer.String(), "token exchanged successfully")
		})
	})
	s.mcpClient.Close()
	s.mcpClient = nil
	s.StopServer()
	s.Require().NoError(s.WaitForShutdown())
}

func TestAuthorization(t *testing.T) {
	suite.Run(t, new(AuthorizationSuite))
}
