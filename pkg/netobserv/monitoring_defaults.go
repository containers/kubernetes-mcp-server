package netobserv

import (
	"net"
	"net/url"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"k8s.io/client-go/rest"
)

// useOpenShiftMonitoringDefaults is true when direct Thanos/Alertmanager URLs are reachable
// from the MCP process (in-cluster). Workstation setups with a port-forwarded plugin URL must
// set prometheus_url / alertmanager_url explicitly instead.
func useOpenShiftMonitoringDefaults(clusterProvider api.ClusterProvider, nc *Config, isOpenShift bool) bool {
	if !isOpenShift {
		return false
	}
	if nc != nil && isLocalPluginURL(nc.Url) {
		return false
	}
	if clusterProvider != nil && strings.TrimSpace(clusterProvider.GetKubeConfigPath()) != "" {
		return false
	}
	_, err := rest.InClusterConfig()
	return err == nil
}

func isLocalPluginURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	return false
}

func monitoringFallbackHint(endpoint string) string {
	switch endpoint {
	case pluginAlertmanagerSilencesPath:
		return "set alertmanager_url in [toolset_configs.netobserv] (e.g. port-forward alertmanager-main 9094:9094 -n openshift-monitoring)"
	case pluginPrometheusRulesPath:
		return "set prometheus_url in [toolset_configs.netobserv] (e.g. port-forward thanos-querier 9091:9091 -n openshift-monitoring)"
	default:
		return "set prometheus_url or alertmanager_url in [toolset_configs.netobserv]"
	}
}
