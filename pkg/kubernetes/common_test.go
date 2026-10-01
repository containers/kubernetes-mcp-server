package kubernetes

import (
	"os"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
)

func TestMain(m *testing.M) {
	// Set up
	_ = os.Setenv("KUBECONFIG", "/dev/null")     // Avoid interference from existing kubeconfig
	_ = os.Setenv("KUBERNETES_SERVICE_HOST", "") // Avoid interference from in-cluster config
	_ = os.Setenv("KUBERNETES_SERVICE_PORT", "") // Avoid interference from in-cluster config

	// Run tests
	code := m.Run()

	if err := test.StopEnvTest(); err != nil {
		panic(err)
	}
	os.Exit(code)
}
