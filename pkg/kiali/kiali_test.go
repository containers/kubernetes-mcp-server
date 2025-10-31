package kiali

import (
    "context"
    "net/http"
    "net/http/httptest"
    "net/url"
    "testing"

    "github.com/containers/kubernetes-mcp-server/pkg/config"
    internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

func TestValidateAndGetURL_JoinsProperly(t *testing.T) {
    m := NewManager(&config.StaticConfig{KialiURL: "https://kiali.example/"})
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

func TestCurrentAuthorizationHeader_FromContext(t *testing.T) {
    m := NewManager(&config.StaticConfig{KialiURL: "https://kiali.example"})
    k := m.GetKiali()
    ctx := context.WithValue(context.Background(), internalk8s.OAuthAuthorizationHeader, "bearer  abc")
    got := k.CurrentAuthorizationHeader(ctx)
    if got != "Bearer abc" {
        t.Fatalf("expected normalized bearer header, got '%s'", got)
    }
}

func TestCurrentAuthorizationHeader_FromManagerToken(t *testing.T) {
    m := NewManager(&config.StaticConfig{KialiURL: "https://kiali.example"})
    m.BearerToken = "abc"
    k := m.GetKiali()
    got := k.CurrentAuthorizationHeader(context.Background())
    if got != "Bearer abc" {
        t.Fatalf("expected 'Bearer abc', got '%s'", got)
    }
}

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

    m := NewManager(&config.StaticConfig{KialiURL: srv.URL})
    k := m.GetKiali()
    ctx := context.WithValue(context.Background(), internalk8s.OAuthAuthorizationHeader, "Bearer token-xyz")

    out, err := k.executeRequest(ctx, "/api/ping?q=1")
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


