package mcp

import (
	"context"
	"fmt"
	"slices"
	"strings"

	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

// checkScopePolicy enforces OAuth scope-based tool authorization.
// Tools listed in scope_policies require the caller's JWT to carry at least one
// matching scope. Tools not listed in any policy are unrestricted (overlay model).
func checkScopePolicy(ctx context.Context, cfg *Configuration, toolName string) error {
	if len(cfg.ScopePolicies) == 0 {
		return nil
	}
	var requiredScopes []string
	for scope, tools := range cfg.ScopePolicies {
		if slices.Contains(tools, toolName) {
			requiredScopes = append(requiredScopes, scope)
		}
	}
	if len(requiredScopes) == 0 {
		return nil
	}
	userScopes, _ := ctx.Value(internalk8s.OAuthScopesKey).([]string)
	for _, us := range userScopes {
		if slices.Contains(requiredScopes, us) {
			return nil
		}
	}
	return fmt.Errorf("insufficient scope: tool %q requires one of scopes: [%s]", toolName, strings.Join(requiredScopes, ", "))
}

// checkResourceScopePolicy enforces OAuth scope-based resource authorization.
// Resources whose URI matches a prefix in resource_scope_policies require the
// caller's JWT to carry at least one matching scope. Resources not matching any
// configured prefix are unrestricted (overlay model).
func checkResourceScopePolicy(ctx context.Context, cfg *Configuration, resourceURI string) error {
	if len(cfg.ResourceScopePolicies) == 0 {
		return nil
	}
	var requiredScopes []string
	for scope, prefixes := range cfg.ResourceScopePolicies {
		for _, prefix := range prefixes {
			if strings.HasPrefix(resourceURI, prefix) {
				requiredScopes = append(requiredScopes, scope)
				break
			}
		}
	}
	if len(requiredScopes) == 0 {
		return nil
	}
	userScopes, _ := ctx.Value(internalk8s.OAuthScopesKey).([]string)
	for _, us := range userScopes {
		if slices.Contains(requiredScopes, us) {
			return nil
		}
	}
	return fmt.Errorf("insufficient scope: resource %q requires one of scopes: [%s]", resourceURI, strings.Join(requiredScopes, ", "))
}
