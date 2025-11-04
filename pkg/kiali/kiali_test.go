package kiali

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

func TestValidateAndGetURL_JoinsProperly(t *testing.T) {
	m := NewManager(&config.StaticConfig{KialiOptions: config.KialiOptions{Url: "https://kiali.example/"}})
	k := m.GetKiali()

	full, err := k.validateAndGetURL("/api/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if full != "https://kiali.example/api/path" {
		t.Fatalf("unexpected url: %s", full)
	}

	m.KialiURL = "https://kiali.example"
	full, err = k.validateAndGetURL("api/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if full != "https://kiali.example/api/path" {
		t.Fatalf("unexpected url: %s", full)
	}

	// preserve query
	m.KialiURL = "https://kiali.example"
	full, err = k.validateAndGetURL("/api/path?x=1&y=2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	u, _ := url.Parse(full)
	if u.Path != "/api/path" || u.Query().Get("x") != "1" || u.Query().Get("y") != "2" {
		t.Fatalf("unexpected parsed url: %s", full)
	}
}

// CurrentAuthorizationHeader behavior is now implicit via executeRequest using Manager.BearerToken

func TestExecuteRequest_SetsAuthAndCallsServer(t *testing.T) {
	// setup test server to assert path and auth header
	var seenAuth string
	var seenPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenPath = r.URL.String()
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	m := NewManager(&config.StaticConfig{KialiOptions: config.KialiOptions{Url: srv.URL}})
	m.BearerToken = "token-xyz"
	k := m.GetKiali()
	out, err := k.executeRequest(context.Background(), "/api/ping?q=1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "ok" {
		t.Fatalf("unexpected body: %s", out)
	}
	if seenAuth != "Bearer token-xyz" {
		t.Fatalf("expected auth header to be set, got '%s'", seenAuth)
	}
	if seenPath != "/api/ping?q=1" {
		t.Fatalf("unexpected path: %s", seenPath)
	}
}
