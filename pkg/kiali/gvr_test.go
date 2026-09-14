package kiali

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

func kialiTestConfig(url string) *config.Config {
	cfg, err := config.ReadToml(context.Background(), []byte(fmt.Sprintf(`
		[toolset_configs.kiali]
		url = "%s"
	`, url)))
	if err != nil {
		panic(err)
	}
	return cfg
}

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

		result := probeStatusURL(context.Background(), srv.URL, &Config{Url: srv.URL}, "")
		if result == nil || !*result {
			t.Fatal("expected probe to succeed")
		}
	})

	t.Run("returns false for missing status object", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"externalServices":[]}`))
		}))
		defer srv.Close()
		result := probeStatusURL(context.Background(), srv.URL, nil, "")
		if result == nil || *result {
			t.Fatal("expected probe to fail")
		}
	})

	t.Run("returns false on connection error", func(t *testing.T) {
		result := probeStatusURL(context.Background(), "http://127.0.0.1:1", nil, "")
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

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		result := probeStatusURL(ctx, "http://"+listener.Addr().String(), nil, "")
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

		if !HasKiali(context.Background(), kialiTestConfig(srv.URL), nil) {
			t.Fatal("expected HasKiali true for reachable configured URL")
		}
	})

	t.Run("disables when configured URL fails status probe", func(t *testing.T) {
		if HasKiali(context.Background(), kialiTestConfig("http://127.0.0.1:1"), nil) {
			t.Fatal("expected HasKiali false for unreachable configured URL")
		}
	})

	t.Run("disables when URL is not configured", func(t *testing.T) {
		if HasKiali(context.Background(), config.New(), nil) {
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

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		if HasKiali(ctx, kialiTestConfig("http://"+listener.Addr().String()), nil) {
			t.Fatal("expected HasKiali false on probe timeout")
		}
	})
}
