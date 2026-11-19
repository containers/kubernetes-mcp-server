package discovery

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDNSName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Already valid names pass through unchanged.
		{"tempo-simplest-query-frontend", "tempo-simplest-query-frontend"},
		{"tempo-simplest", "tempo-simplest"},
		{"abc123", "abc123"},
		// Uppercase letters are lowercased.
		{"TempoStack", "tempostack"},
		// Special characters in the middle become dashes.
		{"tempo_simplest", "tempo-simplest"},
		{"tempo.simplest", "tempo-simplest"},
		// Leading/trailing invalid chars become 'a'.
		{"_leading", "aleading"},
		{"trailing_", "trailinga"},
		{"_both_", "abotha"},
		// Underscores in service name patterns (tempo-operator uses "tempo-<name>-query-frontend").
		{"tempo-my_stack-query-frontend", "tempo-my-stack-query-frontend"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			require.Equal(t, tt.expected, DNSName(tt.input))
		})
	}
}
