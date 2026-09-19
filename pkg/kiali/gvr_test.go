package kiali

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"k8s.io/client-go/rest"
)

type fakeBaseConfig struct {
	configs map[string]api.ExtendedConfig
}

func (f *fakeBaseConfig) GetProviderConfig(string) (api.ExtendedConfig, bool) { return nil, false }
func (f *fakeBaseConfig) GetToolsetConfig(name string) (api.ExtendedConfig, bool) {
	cfg, ok := f.configs[name]
	return cfg, ok
}
func (f *fakeBaseConfig) GetClusterAuthMode() string                      { return "" }
func (f *fakeBaseConfig) ResolveClusterAuthMode() string                  { return "" }
func (f *fakeBaseConfig) GetClusterProviderStrategy() string              { return "" }
func (f *fakeBaseConfig) GetKubeConfigPath() string                       { return "" }
func (f *fakeBaseConfig) GetDeniedResources() []api.GroupVersionKind      { return nil }
func (f *fakeBaseConfig) GetTokenExchangeConfig() api.TokenExchangeConfig { return nil }
func (f *fakeBaseConfig) GetCertificateAuthority() string                 { return "" }
func (f *fakeBaseConfig) IsValidationEnabled() bool                       { return false }
func (f *fakeBaseConfig) IsTargetCompatibilityToolFiltersEnabled() bool   { return true }
func (f *fakeBaseConfig) IsRequireTLS() bool                              { return false }
func (f *fakeBaseConfig) GetTLSMinVersionConfig() string                  { return "" }
func (f *fakeBaseConfig) GetTLSCipherSuitesConfig() []string              { return nil }
func (f *fakeBaseConfig) IsRequireOAuth() bool                            { return false }
func (f *fakeBaseConfig) GetConfirmationRules() []api.ConfirmationRule    { return nil }
func (f *fakeBaseConfig) GetConfirmationFallback() string                 { return "" }

func TestProbeStatusURL(t *testing.T) {
	t.Run("returns true for valid status payload", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/status" {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": map[string]any{"Kiali state": "running"},
			})
		}))
		defer srv.Close()

		cfg := &fakeBaseConfig{configs: map[string]api.ExtendedConfig{
			"kiali": &Config{Url: srv.URL},
		}}
		result := probeStatusURL(context.Background(), cfg, &rest.Config{})
		if result == nil || !*result {
			t.Fatal("expected probe to succeed")
		}
	})

	t.Run("returns false for missing status object", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"externalServices":[]}`))
		}))
		defer srv.Close()
		cfg := &fakeBaseConfig{configs: map[string]api.ExtendedConfig{
			"kiali": &Config{Url: srv.URL},
		}}
		result := probeStatusURL(context.Background(), cfg, &rest.Config{})
		if result == nil || *result {
			t.Fatal("expected probe to fail")
		}
	})

	t.Run("returns false on connection error", func(t *testing.T) {
		cfg := &fakeBaseConfig{configs: map[string]api.ExtendedConfig{
			"kiali": &Config{Url: "http://127.0.0.1:1"},
		}}
		result := probeStatusURL(context.Background(), cfg, &rest.Config{})
		if result == nil || *result {
			t.Fatal("expected probe to fail for unreachable URL")
		}
	})

	t.Run("returns false on probe timeout", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = listener.Close() }()

		go func() {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			time.Sleep(2 * time.Second)
			_ = conn.Close()
		}()

		cfg := &fakeBaseConfig{configs: map[string]api.ExtendedConfig{
			"kiali": &Config{Url: "http://" + listener.Addr().String()},
		}}
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		result := probeStatusURL(ctx, cfg, &rest.Config{})
		if result == nil || *result {
			t.Fatal("expected probe to fail on timeout")
		}
	})

	t.Run("returns nil on temporary DNS error to fail open", func(t *testing.T) {
		if !isTemporaryDNSProbeError(&net.DNSError{IsTemporary: true}) {
			t.Fatal("expected temporary DNS error to fail open")
		}
		if isTemporaryDNSProbeError(context.DeadlineExceeded) {
			t.Fatal("expected timeout not to fail open")
		}
	})
}

func TestHasKiali_ConfiguredURL(t *testing.T) {
	t.Run("enables when configured URL passes status probe", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": map[string]any{"Kiali state": "running"},
			})
		}))
		defer srv.Close()

		cfg := &fakeBaseConfig{configs: map[string]api.ExtendedConfig{
			"kiali": &Config{Url: srv.URL},
		}}
		if !HasKiali(context.Background(), cfg, "") {
			t.Fatal("expected HasKiali true for reachable configured URL")
		}
	})

	t.Run("disables when configured URL fails status probe", func(t *testing.T) {
		cfg := &fakeBaseConfig{configs: map[string]api.ExtendedConfig{
			"kiali": &Config{Url: "http://127.0.0.1:1"},
		}}
		if HasKiali(context.Background(), cfg, "") {
			t.Fatal("expected HasKiali false for unreachable configured URL")
		}
	})

	t.Run("disables when URL is not configured", func(t *testing.T) {
		cfg := &fakeBaseConfig{configs: map[string]api.ExtendedConfig{
			"kiali": &Config{},
		}}
		if HasKiali(context.Background(), cfg, "") {
			t.Fatal("expected HasKiali false without configured URL")
		}
	})

	t.Run("disables on probe timeout", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = listener.Close() }()

		go func() {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			time.Sleep(2 * time.Second)
			_ = conn.Close()
		}()

		cfg := &fakeBaseConfig{configs: map[string]api.ExtendedConfig{
			"kiali": &Config{Url: "http://" + listener.Addr().String()},
		}}
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		if HasKiali(ctx, cfg, "") {
			t.Fatal("expected HasKiali false on probe timeout")
		}
	})
}
