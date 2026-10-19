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

	"k8s.io/client-go/rest"

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

func testKialiClient(t *testing.T, cfg *config.Config) *Kiali {
	k, err := NewKiali(cfg, &rest.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestKiali_Available(t *testing.T) {
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

		k := testKialiClient(t, kialiTestConfig(srv.URL))
		if !k.Available(context.Background()) {
			t.Fatal("expected probe to succeed")
		}
	})

	t.Run("respects the configured backend response limit", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"status":{"Kiali state":"running"}}`))
		}))
		defer srv.Close()

		cfg := kialiTestConfig(srv.URL)
		cfg.MaxBackendResponseBytes.SetForTest(8)
		k := testKialiClient(t, cfg)
		if k.Available(context.Background()) {
			t.Fatal("expected probe to reject a response exceeding the configured limit")
		}
	})

	t.Run("returns false for missing status object", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"externalServices":[]}`))
		}))
		defer srv.Close()
		k := testKialiClient(t, kialiTestConfig(srv.URL))
		if k.Available(context.Background()) {
			t.Fatal("expected probe to fail")
		}
	})

	t.Run("returns false on connection error", func(t *testing.T) {
		k := testKialiClient(t, kialiTestConfig("http://127.0.0.1:1"))
		if k.Available(context.Background()) {
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
		k := testKialiClient(t, kialiTestConfig("http://"+listener.Addr().String()))
		if k.Available(ctx) {
			t.Fatal("expected probe to fail on timeout")
		}
	})

	t.Run("returns false when URL is not configured", func(t *testing.T) {
		k := testKialiClient(t, config.New())
		if k.Available(context.Background()) {
			t.Fatal("expected unavailable without configured URL")
		}
	})

	t.Run("returns true on temporary DNS error to fail open", func(t *testing.T) {
		if !isTemporaryDNSProbeError(&net.DNSError{IsTemporary: true}) {
			t.Fatal("expected temporary DNS error to fail open")
		}
		if isTemporaryDNSProbeError(context.DeadlineExceeded) {
			t.Fatal("expected timeout not to fail open")
		}
	})
}
