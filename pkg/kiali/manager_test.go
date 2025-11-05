package kiali

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

func TestNewManagerUsesConfigFields(t *testing.T) {
	cfg := config.Default()
	cfg.SetToolsetConfig("kiali", &Config{Url: "https://kiali.example", Insecure: true})
	m := NewManager(cfg)
	if m == nil {
		t.Fatalf("expected manager, got nil")
	}
	if m.KialiURL != "https://kiali.example" {
		t.Fatalf("expected KialiURL %s, got %s", "https://kiali.example", m.KialiURL)
	}
	if m.KialiInsecure != true {
		t.Fatalf("expected KialiInsecure %v, got %v", true, m.KialiInsecure)
	}
}

func TestDerivedWithoutAuthorizationReturnsOriginalManager(t *testing.T) {
	cfg := config.Default()
	cfg.SetToolsetConfig("kiali", &Config{Url: "https://kiali.example"})
	m := NewManager(cfg)
	k, err := m.Derived(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if k == nil || k.manager != m {
		t.Fatalf("expected derived Kiali to keep original manager")
	}
}

func TestDerivedPreservesURLAndToken(t *testing.T) {
	cfg := config.Default()
	cfg.SetToolsetConfig("kiali", &Config{Url: "https://kiali.example", Insecure: true})
	m := NewManager(cfg)
	m.BearerToken = "token-abc"
	k, err := m.Derived(context.Background())
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
