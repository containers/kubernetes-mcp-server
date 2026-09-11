package olm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// filteringProvider for testing tool compatibility filters
type filteringProvider struct {
	enabled bool
	has     map[schema.GroupVersionKind]bool
}

func (f filteringProvider) IsTargetCompatibilityToolFiltersEnabled() bool { return f.enabled }
func (f filteringProvider) AnyTargetHasGVKs(_ context.Context, gvks []schema.GroupVersionKind) bool {
	for _, gvk := range gvks {
		if f.has[gvk] {
			return true
		}
	}
	return false
}

// TestToolsetMetadata verifies basic toolset structure
func TestToolsetMetadata(t *testing.T) {
	tools := (&Toolset{}).GetTools(filteringProvider{enabled: true, has: map[schema.GroupVersionKind]bool{
		SubscriptionGVK:     true,
		CatalogSourceGVK:    true,
		ClusterExtensionGVK: true,
		ClusterCatalogGVK:   true,
	}})
	require.Len(t, tools, 5)

	expectedNames := []string{
		"olm_diagnose",
		"olm_diagnose_installation", "olm_assess_namespace", "olm_analyze_condition", "olm_catalog_inspect",
	}

	for i, tool := range tools {
		require.Equal(t, expectedNames[i], tool.Tool.Name)
		require.NotNil(t, tool.Tool.InputSchema)
		require.Equal(t, "object", tool.Tool.InputSchema.Type)
		require.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		require.True(t, *tool.Tool.Annotations.ReadOnlyHint)
		require.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		require.False(t, *tool.Tool.Annotations.DestructiveHint)
		require.Len(t, tool.TargetCompatibilityFilters, 1)
	}
}

// TestToolsetFiltersOutWithoutOLMAPI verifies filters work when OLM APIs unavailable
func TestToolsetFiltersOutWithoutOLMAPI(t *testing.T) {
	tools := (&Toolset{}).GetTools(filteringProvider{enabled: true, has: map[schema.GroupVersionKind]bool{}})
	for _, tool := range tools {
		if len(tool.TargetCompatibilityFilters) > 0 {
			require.False(t, tool.TargetCompatibilityFilters[0](), tool.Tool.Name)
		}
	}
}

// TestToolsetFiltersDisabledRemainVisible verifies tools visible when filtering disabled
func TestToolsetFiltersDisabledRemainVisible(t *testing.T) {
	tools := (&Toolset{}).GetTools(filteringProvider{enabled: false, has: map[schema.GroupVersionKind]bool{}})
	for _, tool := range tools {
		require.GreaterOrEqual(t, len(tool.TargetCompatibilityFilters), 1, tool.Tool.Name)
	}
}
