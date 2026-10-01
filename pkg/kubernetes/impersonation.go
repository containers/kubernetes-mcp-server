package kubernetes

import (
	"context"
	"errors"
	"slices"
	"strings"
	"unicode"
)

// ImpersonationIdentityTokenInfoKey carries the proxy-verified identity through MCP request metadata.
const ImpersonationIdentityTokenInfoKey = "kubernetes-mcp-server/impersonation"

// ImpersonationIdentity is supplied by a trusted authenticating proxy, never by tool arguments.
type ImpersonationIdentity struct {
	UserName string
	Groups   []string
}

func (i ImpersonationIdentity) Validate() error {
	valid := func(value string) bool {
		return value != "" && strings.TrimSpace(value) == value && !strings.ContainsFunc(value, unicode.IsControl)
	}
	if !valid(i.UserName) {
		return errors.New("a valid impersonation user is required")
	}
	for _, group := range i.Groups {
		if !valid(group) {
			return errors.New("invalid impersonation group")
		}
	}
	return nil
}

type impersonationIdentityKey struct{}

// WithImpersonationIdentity replaces any identity inherited from a session context.
func WithImpersonationIdentity(ctx context.Context, identity ImpersonationIdentity) context.Context {
	identity.Groups = slices.Clone(identity.Groups)
	return context.WithValue(ctx, impersonationIdentityKey{}, identity)
}

func ImpersonationIdentityFromContext(ctx context.Context) (ImpersonationIdentity, bool) {
	identity, ok := ctx.Value(impersonationIdentityKey{}).(ImpersonationIdentity)
	identity.Groups = slices.Clone(identity.Groups)
	return identity, ok
}
