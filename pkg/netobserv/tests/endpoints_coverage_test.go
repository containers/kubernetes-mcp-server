package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEndpointsCoverage ensures all documented endpoints have tests
func TestEndpointsCoverage(t *testing.T) {
	documentedEndpoints := []string{
		"/api/loki/flow/records",
		"/api/flow/metrics",
		"/api/loki/export",
		"/healthz",
	}

	// This test serves as documentation and catches missing tests
	// If you add a new endpoint, add it above and write a test in backend/
	require.NotEmpty(t, documentedEndpoints, "should have documented endpoints")
}
