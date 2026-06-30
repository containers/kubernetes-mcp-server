package kubernetes

import (
	"os"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
)

func TestMain(m *testing.M) {
	// Initialize shared envtest for this package
	test.EnvTest()

	// Run tests
	code := m.Run()

	// Clean up
	_ = test.StopEnvTest()
	os.Exit(code)
}
