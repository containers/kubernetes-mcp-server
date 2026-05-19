package netobserv

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
)

type NetObservSuite struct {
	suite.Suite
	MockServer *test.MockServer
	Config     *config.StaticConfig
}

func (s *NetObservSuite) SetupTest() {
	s.MockServer = test.NewMockServer()
	s.MockServer.Config().BearerToken = ""
	s.Config = config.Default()
}

func (s *NetObservSuite) TearDownTest() {
	s.MockServer.Close()
}

func (s *NetObservSuite) TestNewNetObserv_SetsFields() {
	s.Config = test.Must(config.ReadToml([]byte(`
		[toolset_configs.netobserv]
		url = "https://netobserv.example/"
		insecure = true
	`)))
	client := NewNetObserv(s.Config, nil)
	client.bearerToken = "bearer-token"

	s.Equal("https://netobserv.example/", client.pluginURL)
	s.True(client.insecure)
	s.Equal("bearer-token", client.bearerToken)
}

func (s *NetObservSuite) TestExecuteGet() {
	var seenAuth string
	var seenPath string
	var seenQuery url.Values
	s.MockServer.Config().BearerToken = "token-xyz"
	s.MockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenPath = r.URL.Path
		seenQuery = r.URL.Query()
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	s.Config = test.Must(config.ReadToml([]byte(fmt.Sprintf(`
		[toolset_configs.netobserv]
		url = "%s"
	`, s.MockServer.Config().Host))))
	client := NewNetObserv(s.Config, nil)
	client.bearerToken = "token-xyz"

	content, err := client.ExecuteGet(s.T().Context(), "/api/loki/flow/records", map[string]any{
		"namespace": "default",
		"timeRange": 300,
		"limit":     50,
	})
	s.Require().NoError(err)
	s.Equal(`{"status":"ok"}`, content)
	s.Equal("Bearer token-xyz", seenAuth)
	s.Equal("/api/loki/flow/records", seenPath)
	s.Equal("default", seenQuery.Get("namespace"))
	s.Equal("300", seenQuery.Get("timeRange"))
	s.Equal("50", seenQuery.Get("limit"))

	s.Run("returns error when response exceeds maximum allowed size", func() {
		s.MockServer.ResetHandlers()
		s.MockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(strings.Repeat("x", maxResponseBodySize+1)))
		}))
		_, err := client.ExecuteGet(s.T().Context(), "/api/loki/flow/records", nil)
		s.Require().Error(err)
		s.ErrorContains(err, fmt.Sprintf("exceeded maximum allowed size of %d bytes", maxResponseBodySize))
	})
}

func (s *NetObservSuite) TestExecuteGetAccept_csv() {
	var seenAccept string
	s.MockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAccept = r.Header.Get("Accept")
		_, _ = w.Write([]byte("col1,col2\na,b"))
	}))
	s.Config = test.Must(config.ReadToml([]byte(fmt.Sprintf(`
		[toolset_configs.netobserv]
		url = "%s"
	`, s.MockServer.Config().Host))))
	client := NewNetObserv(s.Config, nil)

	content, err := client.ExecuteGetAccept(s.T().Context(), "/api/loki/export", map[string]any{
		"format": "csv",
	}, "text/csv,*/*")
	s.Require().NoError(err)
	s.Equal("col1,col2\na,b", content)
	s.Equal("text/csv,*/*", seenAccept)
}

func (s *NetObservSuite) TestNewNetObserv_usesDefaultURLWithoutConfigSection() {
	client := NewNetObserv(s.Config, nil)
	s.Equal(DefaultPluginURL(false), client.pluginURL)
}

func (s *NetObservSuite) TestRequireTLS_ConfigValidation() {
	s.Run("rejects HTTP URL when require_tls is enabled", func() {
		_, err := config.ReadToml([]byte(`
			require_tls = true
			[toolset_configs.netobserv]
			url = "http://netobserv.example/"
			insecure = true
		`))
		s.Require().Error(err)
		s.ErrorContains(err, "require_tls is enabled but NetObserv URL uses \"http\" scheme")
	})

	s.Run("accepts HTTPS URL when require_tls is enabled", func() {
		tempDir := s.T().TempDir()
		caFile := filepath.Join(tempDir, "ca.crt")
		s.Require().NoError(os.WriteFile(caFile, []byte("test ca content"), 0644))
		cfg, err := config.ReadToml([]byte(`
			require_tls = true
			[toolset_configs.netobserv]
			url = "https://netobserv.example/"
			insecure = false
			certificate_authority = "` + filepath.ToSlash(caFile) + `"
		`))
		s.Require().NoError(err)
		s.NotNil(cfg)
	})
}

func TestNetObserv(t *testing.T) {
	suite.Run(t, new(NetObservSuite))
}
