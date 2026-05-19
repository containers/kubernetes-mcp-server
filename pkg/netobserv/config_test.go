package netobserv

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/stretchr/testify/suite"
)

type ConfigSuite struct {
	suite.Suite
}

func (s *ConfigSuite) TestResolvedURL_defaults() {
	s.Equal(DefaultPluginURL(false), (&Config{}).ResolvedURL(false))
	s.Equal(DefaultPluginURL(true), (&Config{}).ResolvedURL(true))
}

func (s *ConfigSuite) TestResolvedURL_explicitURL() {
	s.Equal("https://custom.example/", (&Config{Url: "https://custom.example/"}).ResolvedURL(false))
}

func (s *ConfigSuite) TestResolvedURL_namespaceOverride() {
	cfg := &Config{Namespace: "openshift-netobserv"}
	s.Equal(BuildPluginURL("openshift-netobserv", DefaultPluginService, DefaultPluginPort, true), cfg.ResolvedURL(true))
}

func (s *ConfigSuite) TestReadToml_emptySectionUsesDefaults() {
	cfg, err := config.ReadToml([]byte(`
		toolsets = ["netobserv"]
		[toolset_configs.netobserv]
	`))
	s.Require().NoError(err)
	nc, ok := cfg.GetToolsetConfig("netobserv")
	s.Require().True(ok)
	netobservCfg := nc.(*Config)
	s.Equal(DefaultPluginURL(false), netobservCfg.ResolvedURL(false))
	s.False(netobservCfg.Insecure)
}

func (s *ConfigSuite) TestNewNetObserv_withoutToolsetConfigSection() {
	base := config.BaseDefault()
	base.Toolsets = append(base.Toolsets, "netobserv")
	client := NewNetObserv(base, nil)
	s.Equal(DefaultPluginURL(false), client.pluginURL)
	s.False(client.insecure)
}

func (s *ConfigSuite) TestApplyDefaults_explicitURLUnchanged() {
	cfg := &Config{Url: "http://localhost:9001"}
	cfg.applyDefaults(false, true)
	s.False(cfg.Insecure)
	s.Empty(cfg.CertificateAuthority)
}

func (s *ConfigSuite) TestApplyDefaults_skipsTLSOnNonOpenShift() {
	cfg := &Config{}
	cfg.applyDefaults(false, false)
	s.False(cfg.Insecure)
	s.Empty(cfg.CertificateAuthority)
}

func (s *ConfigSuite) TestApplyDefaults_usesServiceCAWhenPresent() {
	caFile := filepath.Join(s.T().TempDir(), "service-ca.crt")
	s.Require().NoError(os.WriteFile(caFile, []byte("test ca"), 0644))
	cfg := &Config{}
	cfg.applyDefaultsWithStat(false, true, func(path string) (os.FileInfo, error) {
		if path == DefaultPluginServiceCAPath {
			return os.Stat(caFile)
		}
		return nil, os.ErrNotExist
	})
	s.Equal(DefaultPluginServiceCAPath, cfg.CertificateAuthority)
	s.False(cfg.Insecure)
}

func (s *ConfigSuite) TestApplyDefaults_fallsBackToInsecureWithoutServiceCA() {
	cfg := &Config{}
	cfg.applyDefaultsWithStat(false, true, func(string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	})
	s.True(cfg.Insecure)
	s.Empty(cfg.CertificateAuthority)
}

func (s *ConfigSuite) TestResolvedMonitoringURLs() {
	s.Run("OpenShift defaults when enabled", func() {
		cfg := &Config{}
		s.Equal(DefaultOpenShiftPrometheusURL, cfg.ResolvedPrometheusURL(true))
		s.Equal(DefaultOpenShiftAlertmanagerURL, cfg.ResolvedAlertmanagerURL(true))
	})

	s.Run("plain Kubernetes has no defaults", func() {
		cfg := &Config{}
		s.Empty(cfg.ResolvedPrometheusURL(false))
		s.Empty(cfg.ResolvedAlertmanagerURL(false))
	})

	s.Run("explicit overrides", func() {
		cfg := &Config{
			PrometheusUrl:   "https://prom.example/",
			AlertmanagerUrl: "https://am.example",
		}
		s.Equal("https://prom.example", cfg.ResolvedPrometheusURL(false))
		s.Equal("https://am.example", cfg.ResolvedAlertmanagerURL(true))
	})
}

func (s *ConfigSuite) TestIsLocalPluginURL() {
	s.True(isLocalPluginURL("http://127.0.0.1:9001"))
	s.True(isLocalPluginURL("https://localhost:9001/"))
	s.False(isLocalPluginURL("https://netobserv-plugin.netobserv.svc.cluster.local:9001"))
	s.False(isLocalPluginURL(""))
}

func TestConfig(t *testing.T) {
	suite.Run(t, new(ConfigSuite))
}
