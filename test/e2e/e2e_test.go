//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

var testenv env.Environment

func TestMain(m *testing.M) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = findKubeconfig()
	}

	if kubeconfig != "" {
		testenv = env.NewWithKubeConfig(kubeconfig)
	} else {
		testenv = env.New()
	}

	testenv.Setup(logClusterConnectivity())

	os.Exit(testenv.Run(m))
}

func findKubeconfig() string {
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))

	candidate := filepath.Join(repoRoot, "_output", "kubeconfig")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}

func logClusterConnectivity() env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		clientset, err := clientsetFromKubeconfig(cfg.KubeconfigFile())
		if err != nil {
			fmt.Fprintf(os.Stderr, "[e2e %s] WARNING: cannot connect to cluster: %v\n", time.Now().Format("15:04:05"), err)
			fmt.Fprintf(os.Stderr, "[e2e %s] Tests requiring a cluster will fail. Run 'make e2e-setup' first.\n", time.Now().Format("15:04:05"))
			return ctx, nil
		}
		v, err := clientset.Discovery().ServerVersion()
		if err != nil {
			fmt.Fprintf(os.Stderr, "[e2e %s] WARNING: cluster not reachable: %v\n", time.Now().Format("15:04:05"), err)
			return ctx, nil
		}
		fmt.Fprintf(os.Stderr, "[e2e %s] Connected to cluster %s\n", time.Now().Format("15:04:05"), v.GitVersion)
		return ctx, nil
	}
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func serverImage() string {
	return envOrDefault("MCP_SERVER_IMAGE", "localhost/kubernetes-mcp-server:e2e")
}

func chartPath() string {
	abs, err := filepath.Abs("../../charts/kubernetes-mcp-server")
	if err != nil {
		return envOrDefault("CHART_PATH", "../../charts/kubernetes-mcp-server")
	}
	return envOrDefault("CHART_PATH", abs)
}

func helmPath() string {
	return envOrDefault("HELM_PATH", "helm")
}

func kubectlPath() string {
	return envOrDefault("KUBECTL_PATH", "kubectl")
}
