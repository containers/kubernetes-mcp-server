package api

import (
	"context"
	"strings"

	"k8s.io/klog/v2"
)

// ScopeProvider is implemented by types that can provide OAuth scopes.
// This interface allows the MCP layer to check scopes without importing the http package.
type ScopeProvider interface {
	GetScopes() []string
}

// scopeProviderContextKey is the context key for storing a ScopeProvider
type scopeProviderContextKey struct{}

// ScopeProviderContextKey is used to store/retrieve a ScopeProvider from context
var ScopeProviderContextKey = scopeProviderContextKey{}

// GetScopeProviderFromContext retrieves a ScopeProvider from the context.
// Returns nil if no provider is present (e.g., OAuth not enabled or STDIO mode).
func GetScopeProviderFromContext(ctx context.Context) ScopeProvider {
	val := ctx.Value(ScopeProviderContextKey)
	if val == nil {
		return nil
	}
	if provider, ok := val.(ScopeProvider); ok {
		return provider
	}
	klog.Warningf("Context contains unexpected type for ScopeProviderContextKey: %T", val)
	return nil
}

// HasScope checks if the given scopes slice contains the required scope.
// Comparison is case-insensitive to handle IdP variations.
func HasScope(scopes []string, requiredScope string) bool {
	for _, s := range scopes {
		if strings.EqualFold(s, requiredScope) {
			return true
		}
	}
	return false
}
