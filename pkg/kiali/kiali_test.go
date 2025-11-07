package kiali

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
)

type KialiSuite struct {
	suite.Suite
	MockServer *test.MockServer
	Config     *config.StaticConfig
}

func (s *KialiSuite) SetupTest() {
	s.MockServer = test.NewMockServer()
	s.MockServer.Config().BearerToken = ""
	s.Config = config.Default()
}

func (s *KialiSuite) TearDownTest() {
	s.MockServer.Close()
}

func (s *KialiSuite) TestNewKiali_SetsFields() {
	s.Config = test.Must(config.ReadToml([]byte(`
		[toolset_configs.kiali]
		url = "https://kiali.example/"
		insecure = true
	`)))
	s.MockServer.Config().BearerToken = "bearer-token"
	k := NewKiali(s.Config, s.MockServer.Config())

	s.Run("URL is set", func() {
		s.Equal("https://kiali.example/", k.kialiURL, "Unexpected Kiali URL")
	})
	s.Run("Insecure is set", func() {
		s.True(k.kialiInsecure, "Expected Kiali Insecure to be true")
	})
	s.Run("BearerToken is set", func() {
		s.Equal("bearer-token", k.bearerToken, "Unexpected Kiali BearerToken")
	})
}

func (s *KialiSuite) TestNewKiali_InvalidConfig() {
	cfg, err := config.ReadToml([]byte(`
		[toolset_configs.kiali]
		url = "://invalid-url"
	`))
	s.Error(err, "Expected error reading invalid config")
	s.ErrorContains(err, "kiali-url must be a valid URL", "Unexpected error message")
	s.Nil(cfg, "Unexpected Kiali config")
}

func (s *KialiSuite) TestValidateAndGetURL() {
	s.Config = test.Must(config.ReadToml([]byte(`
		[toolset_configs.kiali]
		url = "https://kiali.example/"
	`)))
	k := NewKiali(s.Config, s.MockServer.Config())

	s.Run("Computes full URL", func() {
		s.Run("with leading slash", func() {
			full, err := k.validateAndGetURL("/api/path")
			s.Require().NoError(err, "Expected no error validating URL")
			s.Equal("https://kiali.example/api/path", full, "Unexpected full URL")
		})

		s.Run("without leading slash", func() {
			full, err := k.validateAndGetURL("api/path")
			s.Require().NoError(err, "Expected no error validating URL")
			s.Equal("https://kiali.example/api/path", full, "Unexpected full URL")
		})

		s.Run("with query parameters, preserves query", func() {
			full, err := k.validateAndGetURL("/api/path?x=1&y=2")
			s.Require().NoError(err, "Expected no error validating URL")
			u, err := url.Parse(full)
			s.Require().NoError(err, "Expected to parse full URL")
			s.Equal("/api/path", u.Path, "Unexpected path in parsed URL")
			s.Equal("1", u.Query().Get("x"), "Unexpected query parameter x")
			s.Equal("2", u.Query().Get("y"), "Unexpected query parameter y")
		})
	})
}

// CurrentAuthorizationHeader behavior is now implicit via executeRequest using Manager.BearerToken

func (s *KialiSuite) TestExecuteRequest() {
	// setup test server to assert path and auth header
	var seenAuth string
	var seenPath string
	s.MockServer.Config().BearerToken = "token-xyz"
	s.MockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenPath = r.URL.String()
		_, _ = w.Write([]byte("ok"))
	}))

	s.Config = test.Must(config.ReadToml([]byte(fmt.Sprintf(`
		[toolset_configs.kiali]
		url = "%s"
	`, s.MockServer.Config().Host))))
	k := NewKiali(s.Config, s.MockServer.Config())

	out, err := k.executeRequest(s.T().Context(), "/api/ping?q=1")
	s.Require().NoError(err, "Expected no error executing request")
	s.Run("auth header set", func() {
		s.Equal("Bearer token-xyz", seenAuth, "Unexpected Authorization header")
	})
	s.Run("path is correct", func() {
		s.Equal("/api/ping?q=1", seenPath, "Unexpected path")
	})
	s.Run("response body is correct", func() {
		s.Equal("ok", out, "Unexpected response body")
	})
}

func TestKiali(t *testing.T) {
	suite.Run(t, new(KialiSuite))
}
