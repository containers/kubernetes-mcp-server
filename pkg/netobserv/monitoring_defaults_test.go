package netobserv

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

type stubClusterProvider struct {
	kubeconfig string
}

func (s stubClusterProvider) GetClusterProviderStrategy() string { return "" }
func (s stubClusterProvider) GetKubeConfigPath() string          { return s.kubeconfig }

func TestUseOpenShiftMonitoringDefaults_localPluginURL(t *testing.T) {
	t.Parallel()
	cfg := &Config{Url: "https://127.0.0.1:9001"}
	if useOpenShiftMonitoringDefaults(nil, cfg, true) {
		t.Fatal("expected false when plugin URL is loopback")
	}
}

func TestUseOpenShiftMonitoringDefaults_withKubeconfigPath(t *testing.T) {
	t.Parallel()
	if useOpenShiftMonitoringDefaults(stubClusterProvider{kubeconfig: "/home/user/.kube/config"}, &Config{}, true) {
		t.Fatal("expected false when kubeconfig path is set")
	}
}

func TestUseOpenShiftMonitoringDefaults_notOpenShift(t *testing.T) {
	t.Parallel()
	if useOpenShiftMonitoringDefaults(nil, &Config{}, false) {
		t.Fatal("expected false on plain Kubernetes")
	}
}

func TestNewNetObserv_localPluginSkipsMonitoringDefaults(t *testing.T) {
	t.Parallel()
	cfg, err := config.ReadToml([]byte(`
		toolsets = ["netobserv"]
		[toolset_configs.netobserv]
		url = "https://127.0.0.1:9001"
		insecure = true
	`))
	if err != nil {
		t.Fatalf("ReadToml: %v", err)
	}
	client := NewNetObserv(cfg, nil)
	if client.prometheusURL != "" || client.alertmanagerURL != "" {
		t.Fatalf("expected empty monitoring URLs, got prom=%q am=%q", client.prometheusURL, client.alertmanagerURL)
	}
}
