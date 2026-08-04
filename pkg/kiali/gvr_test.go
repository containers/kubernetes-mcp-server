package kiali

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

type fakeFilteringProvider struct {
	hasGVKs     bool
	queriedGVKs []schema.GroupVersionKind
}

func (f *fakeFilteringProvider) AnyTargetHasGVKs(_ context.Context, gvks []schema.GroupVersionKind) bool {
	f.queriedGVKs = gvks
	return f.hasGVKs
}

func (f *fakeFilteringProvider) IsTargetCompatibilityToolFiltersEnabled() bool { return true }

// fakeFilteringProviderWithConfig implements both FilteringProvider and ExtendedConfigProvider.
type fakeFilteringProviderWithConfig struct {
	fakeFilteringProvider
	configs map[string]api.ExtendedConfig
}

func (f *fakeFilteringProviderWithConfig) GetProviderConfig(string) (api.ExtendedConfig, bool) {
	return nil, false
}

func (f *fakeFilteringProviderWithConfig) GetToolsetConfig(name string) (api.ExtendedConfig, bool) {
	cfg, ok := f.configs[name]
	return cfg, ok
}

func TestHasKiali(t *testing.T) {
	t.Run("queries for Kiali GVK", func(t *testing.T) {
		p := &fakeFilteringProvider{hasGVKs: true}
		filter := HasKiali(p)
		filter()

		if len(p.queriedGVKs) != 1 {
			t.Fatalf("expected 1 GVK query, got %d", len(p.queriedGVKs))
		}
		if p.queriedGVKs[0] != KialiGVK {
			t.Errorf("expected query for %v, got %v", KialiGVK, p.queriedGVKs[0])
		}
	})

	t.Run("returns true when provider has Kiali GVK", func(t *testing.T) {
		filter := HasKiali(&fakeFilteringProvider{hasGVKs: true})
		if !filter() {
			t.Error("expected HasKiali to return true")
		}
	})

	t.Run("returns false when provider does not have Kiali GVK and URL is unset", func(t *testing.T) {
		filter := HasKiali(&fakeFilteringProvider{hasGVKs: false})
		if filter() {
			t.Error("expected HasKiali to return false")
		}
	})

	t.Run("returns true when URL is configured even without Kiali GVK", func(t *testing.T) {
		p := &fakeFilteringProviderWithConfig{
			fakeFilteringProvider: fakeFilteringProvider{hasGVKs: false},
			configs: map[string]api.ExtendedConfig{
				"kiali": &Config{Url: "https://kiali.example"},
			},
		}
		filter := HasKiali(p)
		if !filter() {
			t.Error("expected HasKiali to return true when URL is configured")
		}
		if len(p.queriedGVKs) != 0 {
			t.Errorf("expected no GVK query when URL short-circuits, got %v", p.queriedGVKs)
		}
	})

	t.Run("returns false when URL config is empty", func(t *testing.T) {
		p := &fakeFilteringProviderWithConfig{
			fakeFilteringProvider: fakeFilteringProvider{hasGVKs: false},
			configs: map[string]api.ExtendedConfig{
				"kiali": &Config{Url: "   "},
			},
		}
		filter := HasKiali(p)
		if filter() {
			t.Error("expected HasKiali to return false for blank URL")
		}
	})
}
