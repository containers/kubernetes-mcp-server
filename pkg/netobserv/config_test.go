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
	cfg.applyDefaults(true)
	s.False(cfg.Insecure)
	s.Empty(cfg.CertificateAuthority)
}

func (s *ConfigSuite) TestApplyDefaults_skipsTLSOnNonOpenShift() {
	cfg := &Config{}
	cfg.applyDefaults(false)
	s.False(cfg.Insecure)
	s.Empty(cfg.CertificateAuthority)
}

func (s *ConfigSuite) TestApplyDefaults_usesServiceCAWhenPresent() {
	caFile := filepath.Join(s.T().TempDir(), "service-ca.crt")
	s.Require().NoError(os.WriteFile(caFile, []byte("test ca"), 0644))
	cfg := &Config{}
	cfg.applyDefaultsWithStat(true, func(path string) (os.FileInfo, error) {
		if path == DefaultPluginServiceCAPath {
			return os.Stat(caFile)
		}
		return nil, os.ErrNotExist
	})
	s.Equal(DefaultPluginServiceCAPath, cfg.CertificateAuthority)
	s.False(cfg.Insecure)
}

func (s *ConfigSuite) TestApplyDefaults_leavesTLSUnsetWithoutServiceCA() {
	cfg := &Config{}
	cfg.applyDefaultsWithStat(true, func(string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	})
	s.False(cfg.Insecure)
	s.Empty(cfg.CertificateAuthority)
}

func TestConfig(t *testing.T) {
	suite.Run(t, new(ConfigSuite))
}
