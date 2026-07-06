package netobserv

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		tls_min_version = "1.3"
		tls_cipher_suites = ["TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"]
		[toolset_configs.netobserv]
		url = "https://netobserv.example/"
		insecure = true
	`)))
	client := NewNetObserv(context.Background(), s.Config, nil, nil)

	s.Equal("https://netobserv.example/", client.pluginURL)
	s.True(client.insecure)
	s.Equal("1.3", client.tlsMinVersion)
	s.Equal([]string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"}, client.tlsCipherSuites)
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
	client := NewNetObserv(context.Background(), s.Config, nil, nil)
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
	s.NotEmpty(seenQuery.Get("startTime"))
	s.NotEmpty(seenQuery.Get("endTime"))
	s.Empty(seenQuery.Get("timeRange"))
	s.Equal("50", seenQuery.Get("limit"))

	s.Run("reads bearer token from BearerTokenFile", func() {
		tokenFile := filepath.Join(s.T().TempDir(), "token")
		s.Require().NoError(os.WriteFile(tokenFile, []byte("file-sa-token\n"), 0600))
		s.MockServer.ResetHandlers()
		s.MockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.Equal("Bearer file-sa-token", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		}))
		client := NewNetObserv(context.Background(), s.Config, nil, nil)
		client.bearerTokenFile = tokenFile

		content, err := client.ExecuteGet(s.T().Context(), "/api/loki/flow/records", nil)
		s.Require().NoError(err)
		s.Equal(`{"status":"ok"}`, content)
	})

	s.Run("returns error when JSON response exceeds maximum allowed size", func() {
		s.MockServer.ResetHandlers()
		s.MockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(strings.Repeat("x", maxJSONResponseBodySize+1)))
		}))
		_, err := client.ExecuteGet(s.T().Context(), "/api/loki/flow/records", nil)
		s.Require().Error(err)
		s.ErrorContains(err, fmt.Sprintf("exceeded maximum allowed size of %d bytes", maxJSONResponseBodySize))
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
	client := NewNetObserv(context.Background(), s.Config, nil, nil)

	response, err := client.ExecuteGetAccept(s.T().Context(), "/api/loki/export", map[string]any{
		"format": "csv",
	}, "text/csv,*/*", 2<<20)
	s.Require().NoError(err)
	s.Equal("col1,col2\na,b", response.Body)
	s.False(response.Truncated)
	s.Equal("text/csv,*/*", seenAccept)
}

func (s *NetObservSuite) TestExecuteGetAccept_truncatesLargeExports() {
	s.MockServer.ResetHandlers()
	s.MockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 32)))
	}))
	s.Config = test.Must(config.ReadToml([]byte(fmt.Sprintf(`
		[toolset_configs.netobserv]
		url = "%s"
	`, s.MockServer.Config().Host))))
	client := NewNetObserv(context.Background(), s.Config, nil, nil)

	response, err := client.ExecuteGetAccept(s.T().Context(), "/api/loki/export", nil, "text/csv,*/*", 16)
	s.Require().NoError(err)
	s.True(response.Truncated)
	s.Len(response.Body, 16)
}

func (s *NetObservSuite) TestNewNetObserv_usesDefaultURLWithoutConfigSection() {
	client := NewNetObserv(context.Background(), s.Config, nil, nil)
	s.Equal(DefaultPluginURL(false), client.pluginURL)
}

func (s *NetObservSuite) TestCreateHTTPClient_AppliesTLSSettings() {
	tlsConfigFromClient := func(httpClient *http.Client) *tls.Config {
		s.T().Helper()
		enforcing, ok := httpClient.Transport.(*config.TLSEnforcingTransport)
		s.Require().True(ok, "expected TLSEnforcingTransport wrapper")
		transport, ok := enforcing.Base.(*http.Transport)
		s.Require().True(ok, "expected base http.Transport")
		s.Require().NotNil(transport.TLSClientConfig)
		return transport.TLSClientConfig
	}

	s.Run("uses configured min version and cipher suites", func() {
		s.Config = test.Must(config.ReadToml([]byte(`
			tls_min_version = "1.3"
			tls_cipher_suites = ["TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"]
			[toolset_configs.netobserv]
			url = "https://netobserv.example/"
			insecure = true
		`)))
		client := NewNetObserv(context.Background(), s.Config, nil, nil)

		httpClient, err := client.createHTTPClient(s.T().Context())
		s.Require().NoError(err)
		tlsConfig := tlsConfigFromClient(httpClient)
		s.Equal(uint16(tls.VersionTLS13), tlsConfig.MinVersion)
		s.Equal([]uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}, tlsConfig.CipherSuites)
		s.True(tlsConfig.InsecureSkipVerify)
	})

	s.Run("returns error for invalid TLS min version", func() {
		client := NewNetObserv(context.Background(), s.Config, nil, nil)
		client.tlsMinVersion = "9.9"

		httpClient, err := client.createHTTPClient(s.T().Context())
		s.Error(err, "expected error for invalid TLS min version")
		s.Nil(httpClient, "client should be nil when TLS config fails")
		s.ErrorContains(err, "failed to build TLS config")
	})
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

func (s *NetObservSuite) TestCreateHTTPClient_failsClosedOnInvalidCA() {
	client := NewNetObserv(context.Background(), s.Config, nil, nil)
	client.certificateAuthority = filepath.Join(s.T().TempDir(), "missing-ca.crt")

	_, err := client.createHTTPClient(s.T().Context())
	s.Require().Error(err)
	s.ErrorContains(err, "failed to read certificate authority")
}

func (s *NetObservSuite) TestExecuteGet_rejectsRedirects() {
	redirectTarget := test.NewMockServer()
	redirectTarget.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Fail("redirect target should not be called")
	}))
	s.T().Cleanup(redirectTarget.Close)

	s.MockServer.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.Config().Host+"/stolen", http.StatusFound)
	}))
	s.Config = test.Must(config.ReadToml([]byte(fmt.Sprintf(`
		[toolset_configs.netobserv]
		url = "%s"
	`, s.MockServer.Config().Host))))
	client := NewNetObserv(context.Background(), s.Config, nil, nil)

	_, err := client.ExecuteGet(s.T().Context(), "/api/loki/flow/records", nil)
	s.Require().Error(err)
	s.ErrorContains(err, "redirects are not allowed")
}

func (s *NetObservSuite) TestNewNetObserv_openShiftProviderUsesHTTPSURL() {
	// A provider that reports the OpenShift Project GVK must synthesize an https:// URL so the
	// bearer token is never sent in cleartext. This is also the fail-open direction: on a discovery
	// error AnyTargetHasGVKs returns true, so this same https path is taken instead of leaking over http.
	provider := &mockFilteringProvider{hasGVKs: true}
	client := NewNetObserv(context.Background(), s.Config, nil, provider)
	s.Equal(DefaultPluginURL(true), client.pluginURL)
}

func (s *NetObservSuite) TestCreateHTTPClient_pinsProvidedCA() {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	s.T().Cleanup(srv.Close)
	pinnedCA := filepath.Join(s.T().TempDir(), "pinned-ca.crt")
	s.Require().NoError(os.WriteFile(pinnedCA,
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600))

	// Set the URL and CA directly on the client rather than via TOML: a Windows temp path
	// (C:\Users\...) is not a valid double-quoted TOML string ("\U" is read as an escape).
	s.Run("verifies against the pinned CA", func() {
		client := NewNetObserv(context.Background(), s.Config, nil, nil)
		client.pluginURL = srv.URL
		client.certificateAuthority = pinnedCA

		_, err := client.ExecuteGet(s.T().Context(), "/api/loki/flow/records", nil)
		s.NoError(err)
	})

	s.Run("rejects a server cert not signed by the pinned CA", func() {
		client := NewNetObserv(context.Background(), s.Config, nil, nil)
		client.pluginURL = srv.URL
		client.certificateAuthority = s.writeSelfSignedCA()

		_, err := client.ExecuteGet(s.T().Context(), "/api/loki/flow/records", nil)
		s.Error(err)
	})
}

// writeSelfSignedCA generates a self-signed CA certificate unrelated to the httptest server cert,
// writes it to a temp file, and returns the path. Used to prove the pinned CA is the sole trust anchor.
func (s *NetObservSuite) writeSelfSignedCA() string {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	s.Require().NoError(err)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "unrelated-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	s.Require().NoError(err)
	path := filepath.Join(s.T().TempDir(), "unrelated-ca.crt")
	s.Require().NoError(os.WriteFile(path,
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	return path
}

func TestNetObserv(t *testing.T) {
	suite.Run(t, new(NetObservSuite))
}
