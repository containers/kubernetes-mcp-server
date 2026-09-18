package test

import "github.com/containers/kubernetes-mcp-server/pkg/config"

const (
	EnvtestKubeClientQPS   float32 = 1000
	EnvtestKubeClientBurst int     = 2000
)

// ApplyEnvtestClientLimits sets high Kubernetes client QPS/burst on cfg so
// envtest suites are not client-side throttled. New and BaseDefault ignore
// process env, so these must be SetForTest on the config itself.
func ApplyEnvtestClientLimits(cfg *config.Config) {
	if cfg == nil {
		return
	}
	cfg.KubeClientQPS.SetForTest(EnvtestKubeClientQPS)
	cfg.KubeClientBurst.SetForTest(EnvtestKubeClientBurst)
}
