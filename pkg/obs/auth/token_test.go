package auth

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

func TestReadTokenFromContext(t *testing.T) {
	tests := []struct {
		name     string
		ctxValue any
		want     string
	}{
		{
			name:     "Bearer prefix is stripped",
			ctxValue: "Bearer my-token-123",
			want:     "my-token-123",
		},
		{
			name:     "raw token without prefix returned as-is",
			ctxValue: "my-raw-token",
			want:     "my-raw-token",
		},
		{
			name:     "no value in context returns empty",
			ctxValue: nil,
			want:     "",
		},
		{
			name:     "non-string value in context returns empty",
			ctxValue: 12345,
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			if tt.ctxValue != nil {
				ctx = context.WithValue(ctx, kubernetes.OAuthAuthorizationHeader, tt.ctxValue)
			}
			got := readTokenFromContext(ctx)
			if got != tt.want {
				t.Errorf("readTokenFromCtx() = %q, want %q", got, tt.want)
			}
		})
	}
}
