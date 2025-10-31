package kiali

import (
    "context"
    "testing"

    "github.com/containers/kubernetes-mcp-server/pkg/config"
    internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

func TestNewManagerUsesConfigFields(t *testing.T) {
    cfg := &config.StaticConfig{KialiURL: "https://kiali.example", KialiInsecure: true}
    m := NewManager(cfg)
    if m == nil {
        t.Fatalf("expected manager, got nil")
    }
    if m.KialiURL != cfg.KialiURL {
        t.Fatalf("expected KialiURL %s, got %s", cfg.KialiURL, m.KialiURL)
    }
    if m.KialiInsecure != cfg.KialiInsecure {
        t.Fatalf("expected KialiInsecure %v, got %v", cfg.KialiInsecure, m.KialiInsecure)
    }
}

func TestDerivedWithoutAuthorizationReturnsOriginalManager(t *testing.T) {
    cfg := &config.StaticConfig{KialiURL: "https://kiali.example"}
    m := NewManager(cfg)
    k, err := m.Derived(context.Background())
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if k == nil || k.manager != m {
        t.Fatalf("expected derived Kiali to keep original manager")
    }
}

func TestDerivedWithAuthorizationPreservesURLAndToken(t *testing.T) {
    cfg := &config.StaticConfig{KialiURL: "https://kiali.example", KialiInsecure: true}
    m := NewManager(cfg)
    ctx := context.WithValue(context.Background(), internalk8s.OAuthAuthorizationHeader, "Bearer token-abc")
    k, err := m.Derived(ctx)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if k == nil || k.manager == nil {
        t.Fatalf("expected derived Kiali with manager")
    }
    if k.manager.BearerToken != "token-abc" {
        t.Fatalf("expected bearer token 'token-abc', got '%s'", k.manager.BearerToken)
    }
    if k.manager.KialiURL != m.KialiURL || k.manager.KialiInsecure != m.KialiInsecure {
        t.Fatalf("expected Kiali URL/insecure preserved")
    }
}


