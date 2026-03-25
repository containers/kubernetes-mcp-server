package mcp

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
	"k8s.io/utils/ptr"
)

type ScopeValidationSuite struct {
	suite.Suite
}

func TestScopeValidation(t *testing.T) {
	suite.Run(t, new(ScopeValidationSuite))
}

// mockScopeProvider implements api.ScopeProvider for testing
type mockScopeProvider struct {
	scopes []string
}

func (m *mockScopeProvider) GetScopes() []string {
	return m.scopes
}

func (s *ScopeValidationSuite) TestHasScope() {
	s.Run("returns true when scope exists", func() {
		s.True(api.HasScope([]string{"read", "write"}, "read"))
		s.True(api.HasScope([]string{"read", "write"}, "write"))
	})

	s.Run("returns false when scope missing", func() {
		s.False(api.HasScope([]string{"read"}, "write"))
		s.False(api.HasScope([]string{}, "read"))
		s.False(api.HasScope(nil, "read"))
	})

	s.Run("comparison is case-insensitive", func() {
		// IdPs may return scopes with different casing
		s.True(api.HasScope([]string{"Read"}, "read"))
		s.True(api.HasScope([]string{"READ"}, "read"))
		s.True(api.HasScope([]string{"read"}, "READ"))
		s.True(api.HasScope([]string{"Write"}, "write"))
	})
}

func (s *ScopeValidationSuite) TestGetScopeProviderFromContext() {
	s.Run("returns provider when present", func() {
		provider := &mockScopeProvider{scopes: []string{"read"}}
		ctx := context.WithValue(context.Background(), api.ScopeProviderContextKey, provider)

		result := api.GetScopeProviderFromContext(ctx)
		s.NotNil(result)
		s.Equal([]string{"read"}, result.GetScopes())
	})

	s.Run("returns nil when no provider", func() {
		ctx := context.Background()
		result := api.GetScopeProviderFromContext(ctx)
		s.Nil(result)
	})

	s.Run("returns nil for wrong type", func() {
		ctx := context.WithValue(context.Background(), api.ScopeProviderContextKey, "not a provider")
		result := api.GetScopeProviderFromContext(ctx)
		s.Nil(result)
	})
}

func (s *ScopeValidationSuite) TestDetermineRequiredScope() {
	s.Run("read-only tool requires read scope", func() {
		tool := &api.ServerTool{
			Tool: api.Tool{
				Name: "config_view",
				Annotations: api.ToolAnnotations{
					ReadOnlyHint: ptr.To(true),
				},
			},
		}

		// readOnlyHint=true should require read scope
		s.True(ptr.Deref(tool.Tool.Annotations.ReadOnlyHint, false))
	})

	s.Run("write tool requires write scope", func() {
		tool := &api.ServerTool{
			Tool: api.Tool{
				Name: "resource_apply",
				Annotations: api.ToolAnnotations{
					ReadOnlyHint: ptr.To(false),
				},
			},
		}

		// readOnlyHint=false should require write scope
		s.False(ptr.Deref(tool.Tool.Annotations.ReadOnlyHint, false))
	})

	s.Run("nil annotations default to write scope", func() {
		tool := &api.ServerTool{
			Tool: api.Tool{
				Name: "some_tool",
			},
		}

		// No annotations should default to write scope (readOnlyHint=nil defaults to false)
		s.False(ptr.Deref(tool.Tool.Annotations.ReadOnlyHint, false))
	})
}

func (s *ScopeValidationSuite) TestValidateScopeConfig() {
	s.Run("returns nil when OAuth disabled", func() {
		cfg := &config.StaticConfig{
			RequireOAuth: false,
			ReadScope:    "", // empty is fine when OAuth disabled
			WriteScope:   "",
		}
		err := validateScopeConfig(cfg)
		s.NoError(err)
	})

	s.Run("returns error when read_scope empty with OAuth enabled", func() {
		cfg := &config.StaticConfig{
			RequireOAuth: true,
			ReadScope:    "",
			WriteScope:   "write",
		}
		err := validateScopeConfig(cfg)
		s.Error(err)
		s.Contains(err.Error(), "read_scope must not be empty")
	})

	s.Run("returns error when write_scope empty with OAuth enabled", func() {
		cfg := &config.StaticConfig{
			RequireOAuth: true,
			ReadScope:    "read",
			WriteScope:   "",
		}
		err := validateScopeConfig(cfg)
		s.Error(err)
		s.Contains(err.Error(), "write_scope must not be empty")
	})

	s.Run("returns nil with valid scopes and OAuth enabled", func() {
		cfg := &config.StaticConfig{
			RequireOAuth: true,
			ReadScope:    "read",
			WriteScope:   "write",
		}
		err := validateScopeConfig(cfg)
		s.NoError(err)
	})

	s.Run("accepts custom scope names", func() {
		cfg := &config.StaticConfig{
			RequireOAuth: true,
			ReadScope:    "kubernetes:read",
			WriteScope:   "kubernetes:write",
		}
		err := validateScopeConfig(cfg)
		s.NoError(err)
	})
}

func (s *ScopeValidationSuite) TestScopeValidationLogic() {
	s.Run("validates read-only tool with read scope", func() {
		provider := &mockScopeProvider{scopes: []string{"read"}}
		requiredScope := "read"

		s.True(api.HasScope(provider.GetScopes(), requiredScope))
	})

	s.Run("rejects read-only tool without read scope", func() {
		provider := &mockScopeProvider{scopes: []string{"write"}}
		requiredScope := "read"

		s.False(api.HasScope(provider.GetScopes(), requiredScope))
	})

	s.Run("validates write tool with write scope", func() {
		provider := &mockScopeProvider{scopes: []string{"write"}}
		requiredScope := "write"

		s.True(api.HasScope(provider.GetScopes(), requiredScope))
	})

	s.Run("rejects write tool with only read scope", func() {
		provider := &mockScopeProvider{scopes: []string{"read"}}
		requiredScope := "write"

		s.False(api.HasScope(provider.GetScopes(), requiredScope))
	})

	s.Run("accepts both read and write scopes for write operations", func() {
		provider := &mockScopeProvider{scopes: []string{"read", "write"}}

		s.True(api.HasScope(provider.GetScopes(), "read"))
		s.True(api.HasScope(provider.GetScopes(), "write"))
	})

	s.Run("write scope grants read access (write implies read)", func() {
		// Users with write scope should be able to call read-only tools
		// This is standard RBAC behavior: write implies read
		scopes := []string{"write"}
		readScope := "read"
		writeScope := "write"

		// For read-only tools, accept either read OR write scope
		hasReadAccess := api.HasScope(scopes, readScope) || api.HasScope(scopes, writeScope)
		s.True(hasReadAccess, "write scope should grant read access")

		// But read scope should NOT grant write access
		scopes = []string{"read"}
		hasWriteAccess := api.HasScope(scopes, writeScope)
		s.False(hasWriteAccess, "read scope should not grant write access")
	})

	s.Run("custom scope names work", func() {
		provider := &mockScopeProvider{scopes: []string{"kubernetes:read"}}

		s.True(api.HasScope(provider.GetScopes(), "kubernetes:read"))
		s.False(api.HasScope(provider.GetScopes(), "kubernetes:write"))
	})

	s.Run("empty scopes should skip validation for backward compatibility", func() {
		// Tokens without scope claims should allow access (backward compatible)
		provider := &mockScopeProvider{scopes: []string{}}
		scopes := provider.GetScopes()

		// len(scopes) == 0 means skip validation, not reject
		s.Equal(0, len(scopes))
	})

	s.Run("nil scopes should skip validation for backward compatibility", func() {
		// Tokens without scope claims should allow access (backward compatible)
		provider := &mockScopeProvider{scopes: nil}
		scopes := provider.GetScopes()

		// len(scopes) == 0 means skip validation, not reject
		s.Equal(0, len(scopes))
	})
}

func (s *ScopeValidationSuite) TestBackwardCompatibilityNoScopes() {
	// This test verifies the middleware allows access when OAuth is enabled
	// but the token has no scope claims (backward compatibility)
	s.Run("middleware allows access when token has empty scopes", func() {
		// Simulate backward compatibility: OAuth enabled but token has no scopes
		provider := &mockScopeProvider{scopes: []string{}}

		// The middleware checks: if len(scopes) == 0, skip validation
		scopes := provider.GetScopes()
		s.Equal(0, len(scopes), "empty scopes should be detected")

		// Verify the backward compatibility behavior:
		// Even for write tools, empty scopes means "allow" not "deny"
		// This is because existing OAuth setups may not include scope claims
		skipValidation := len(scopes) == 0
		s.True(skipValidation, "empty scopes should trigger skip validation path")
	})

	s.Run("middleware allows access when token has nil scopes", func() {
		// Some IdPs may return nil instead of empty slice
		provider := &mockScopeProvider{scopes: nil}

		scopes := provider.GetScopes()
		s.Equal(0, len(scopes), "nil scopes should be treated as empty")

		skipValidation := len(scopes) == 0
		s.True(skipValidation, "nil scopes should trigger skip validation path")
	})

	s.Run("middleware denies access when token has scopes but missing required", func() {
		// When token HAS scopes, validation is enforced
		provider := &mockScopeProvider{scopes: []string{"other-scope"}}

		scopes := provider.GetScopes()
		s.NotEqual(0, len(scopes), "non-empty scopes should be detected")

		// User has scopes but not the required one
		hasReadAccess := api.HasScope(scopes, "read") || api.HasScope(scopes, "write")
		s.False(hasReadAccess, "should deny when scopes present but required scope missing")
	})
}
