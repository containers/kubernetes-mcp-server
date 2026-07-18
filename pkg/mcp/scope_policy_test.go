package mcp

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cfgWithPolicies(policies map[string][]string) *Configuration {
	return &Configuration{
		StaticConfig: &config.StaticConfig{
			ScopePolicies: policies,
		},
	}
}

func cfgWithResourcePolicies(policies map[string][]string) *Configuration {
	return &Configuration{
		StaticConfig: &config.StaticConfig{
			ResourceScopePolicies: policies,
		},
	}
}

func ctxWithScopes(scopes []string) context.Context {
	return context.WithValue(context.Background(), internalk8s.OAuthScopesKey, scopes)
}

func TestCheckScopePolicy_NoPolicies(t *testing.T) {
	cfg := cfgWithPolicies(nil)
	err := checkScopePolicy(context.Background(), cfg, "pods_list")
	assert.NoError(t, err)
}

func TestCheckScopePolicy_ToolNotInAnyPolicy(t *testing.T) {
	cfg := cfgWithPolicies(map[string][]string{
		"mcp:read": {"pods_list", "pods_get"},
	})
	ctx := ctxWithScopes([]string{"mcp:read"})
	err := checkScopePolicy(ctx, cfg, "namespaces_list")
	assert.NoError(t, err)
}

func TestCheckScopePolicy_MatchingScope(t *testing.T) {
	cfg := cfgWithPolicies(map[string][]string{
		"mcp:read": {"pods_list", "pods_get"},
	})
	ctx := ctxWithScopes([]string{"mcp:read"})
	err := checkScopePolicy(ctx, cfg, "pods_list")
	assert.NoError(t, err)
}

func TestCheckScopePolicy_NoMatchingScope(t *testing.T) {
	cfg := cfgWithPolicies(map[string][]string{
		"mcp:admin": {"resources_delete"},
	})
	ctx := ctxWithScopes([]string{"mcp:read"})
	err := checkScopePolicy(ctx, cfg, "resources_delete")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient scope")
	assert.Contains(t, err.Error(), "resources_delete")
	assert.Contains(t, err.Error(), "mcp:admin")
}

func TestCheckScopePolicy_NoScopesInToken(t *testing.T) {
	cfg := cfgWithPolicies(map[string][]string{
		"mcp:read": {"pods_list"},
	})
	ctx := ctxWithScopes(nil)
	err := checkScopePolicy(ctx, cfg, "pods_list")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient scope")
}

func TestCheckScopePolicy_NoScopesInContext(t *testing.T) {
	cfg := cfgWithPolicies(map[string][]string{
		"mcp:read": {"pods_list"},
	})
	err := checkScopePolicy(context.Background(), cfg, "pods_list")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient scope")
}

func TestCheckScopePolicy_MultipleScopesUnion(t *testing.T) {
	cfg := cfgWithPolicies(map[string][]string{
		"mcp:read":  {"pods_list", "pods_get"},
		"mcp:admin": {"pods_list", "resources_delete"},
	})
	ctx := ctxWithScopes([]string{"mcp:admin"})
	assert.NoError(t, checkScopePolicy(ctx, cfg, "pods_list"))
	assert.NoError(t, checkScopePolicy(ctx, cfg, "resources_delete"))
	require.Error(t, checkScopePolicy(ctx, cfg, "pods_get"))
}

func TestCheckResourceScopePolicy_NoPolicies(t *testing.T) {
	cfg := cfgWithResourcePolicies(nil)
	err := checkResourceScopePolicy(context.Background(), cfg, "must-gather://current")
	assert.NoError(t, err)
}

func TestCheckResourceScopePolicy_URINotMatchingAnyPrefix(t *testing.T) {
	cfg := cfgWithResourcePolicies(map[string][]string{
		"mcp:mustgather": {"must-gather://"},
	})
	ctx := ctxWithScopes([]string{"mcp:read"})
	err := checkResourceScopePolicy(ctx, cfg, "other://something")
	assert.NoError(t, err)
}

func TestCheckResourceScopePolicy_MatchingScope(t *testing.T) {
	cfg := cfgWithResourcePolicies(map[string][]string{
		"mcp:mustgather": {"must-gather://"},
	})
	ctx := ctxWithScopes([]string{"mcp:mustgather"})
	assert.NoError(t, checkResourceScopePolicy(ctx, cfg, "must-gather://current"))
	assert.NoError(t, checkResourceScopePolicy(ctx, cfg, "must-gather://current/namespaces"))
	assert.NoError(t, checkResourceScopePolicy(ctx, cfg, "must-gather://current/resources/apps/v1/Deployment/ns/name"))
}

func TestCheckResourceScopePolicy_NoMatchingScope(t *testing.T) {
	cfg := cfgWithResourcePolicies(map[string][]string{
		"mcp:mustgather": {"must-gather://"},
	})
	ctx := ctxWithScopes([]string{"mcp:read"})
	err := checkResourceScopePolicy(ctx, cfg, "must-gather://current")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient scope")
	assert.Contains(t, err.Error(), "must-gather://current")
	assert.Contains(t, err.Error(), "mcp:mustgather")
}

func TestCheckResourceScopePolicy_NoScopesInContext(t *testing.T) {
	cfg := cfgWithResourcePolicies(map[string][]string{
		"mcp:mustgather": {"must-gather://"},
	})
	err := checkResourceScopePolicy(context.Background(), cfg, "must-gather://current")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient scope")
}

func TestCheckResourceScopePolicy_MultiplePrefixes(t *testing.T) {
	cfg := cfgWithResourcePolicies(map[string][]string{
		"mcp:mustgather": {"must-gather://"},
		"mcp:etcd":       {"must-gather://current/etcd/"},
	})
	ctx := ctxWithScopes([]string{"mcp:etcd"})
	assert.NoError(t, checkResourceScopePolicy(ctx, cfg, "must-gather://current/etcd/members"))
	require.Error(t, checkResourceScopePolicy(ctx, cfg, "must-gather://current/namespaces"))
}
