package helm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	helmtime "helm.sh/helm/v3/pkg/time"
	"k8s.io/utils/ptr"
)

type HelmSuite struct {
	suite.Suite
}

func (s *HelmSuite) TestSimplify() {
	s.Run("release with chart and info", func() {
		deployed := helmtime.Time{Time: time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)}
		result := simplify(&release.Release{
			Name:      "my-release",
			Namespace: "default",
			Version:   3,
			Chart: &chart.Chart{
				Metadata: &chart.Metadata{
					Name:       "grafana",
					Version:    "7.0.0",
					AppVersion: "10.4.0",
				},
			},
			Info: &release.Info{
				Status:       release.StatusDeployed,
				LastDeployed: deployed,
			},
		})
		s.Require().Len(result, 1)
		s.Equal("my-release", result[0]["name"])
		s.Equal("default", result[0]["namespace"])
		s.Equal(3, result[0]["revision"])
		s.Equal("grafana", result[0]["chart"])
		s.Equal("7.0.0", result[0]["chartVersion"])
		s.Equal("10.4.0", result[0]["appVersion"])
		s.Equal("deployed", result[0]["status"])
		s.Equal(deployed.Format(time.RFC1123Z), result[0]["lastDeployed"])
	})
	s.Run("release with nil chart", func() {
		result := simplify(&release.Release{
			Name:      "no-chart",
			Namespace: "kube-system",
			Version:   1,
			Info: &release.Info{
				Status: release.StatusFailed,
			},
		})
		s.Require().Len(result, 1)
		s.Equal("no-chart", result[0]["name"])
		s.Equal("kube-system", result[0]["namespace"])
		s.Equal(1, result[0]["revision"])
		s.NotContains(result[0], "chart")
		s.NotContains(result[0], "chartVersion")
		s.NotContains(result[0], "appVersion")
		s.Equal("failed", result[0]["status"])
	})
	s.Run("release with nil info", func() {
		result := simplify(&release.Release{
			Name:      "no-info",
			Namespace: "default",
			Version:   1,
			Chart: &chart.Chart{
				Metadata: &chart.Metadata{
					Name:       "nginx",
					Version:    "1.0.0",
					AppVersion: "1.25.0",
				},
			},
		})
		s.Require().Len(result, 1)
		s.Equal("no-info", result[0]["name"])
		s.Equal("nginx", result[0]["chart"])
		s.NotContains(result[0], "status")
		s.NotContains(result[0], "lastDeployed")
	})
	s.Run("release with zero last deployed time", func() {
		result := simplify(&release.Release{
			Name:      "zero-time",
			Namespace: "default",
			Version:   1,
			Info: &release.Info{
				Status: release.StatusDeployed,
			},
		})
		s.Require().Len(result, 1)
		s.Equal("deployed", result[0]["status"])
		s.NotContains(result[0], "lastDeployed")
	})
	s.Run("multiple releases", func() {
		result := simplify(
			&release.Release{Name: "first", Namespace: "ns-1", Version: 1},
			&release.Release{Name: "second", Namespace: "ns-2", Version: 2},
		)
		s.Require().Len(result, 2)
		s.Equal("first", result[0]["name"])
		s.Equal("ns-1", result[0]["namespace"])
		s.Equal("second", result[1]["name"])
		s.Equal("ns-2", result[1]["namespace"])
	})
	s.Run("no releases", func() {
		result := simplify()
		s.Empty(result)
	})
}

func (s *HelmSuite) TestValidateChartReference() {
	s.Run("without config", func() {
		s.Run("allows oci:// scheme", func() {
			s.NoError(validateChartReference("oci://ghcr.io/nginxinc/charts/nginx-ingress", nil))
		})
		s.Run("allows https:// scheme", func() {
			s.NoError(validateChartReference("https://charts.example.com/grafana-7.0.0.tgz", nil))
		})
		s.Run("allows non-URL references", func() {
			s.NoError(validateChartReference("stable/grafana", nil))
		})
		s.Run("allows plain chart name", func() {
			s.NoError(validateChartReference("grafana", nil))
		})
		s.Run("allows Windows drive letter path", func() {
			s.NoError(validateChartReference(`D:\a\kubernetes-mcp-server\testdata\helm-chart`, nil))
		})
		s.Run("blocks file:// scheme", func() {
			err := validateChartReference("file:///tmp/malicious-chart", nil)
			s.Error(err)
			s.Contains(err.Error(), "file:// scheme is blocked")
		})
		s.Run("blocks http:// scheme", func() {
			err := validateChartReference("http://evil.example.com/chart", nil)
			s.Error(err)
			s.Contains(err.Error(), "http:// scheme is blocked")
		})
		s.Run("blocks unknown scheme", func() {
			err := validateChartReference("ftp://example.com/chart", nil)
			s.Error(err)
			s.Contains(err.Error(), "only oci:// and https:// schemes are permitted")
		})
	})
	s.Run("with empty config", func() {
		cfg := &Config{}
		s.Run("allows oci:// scheme", func() {
			s.NoError(validateChartReference("oci://ghcr.io/myorg/chart", cfg))
		})
		s.Run("allows non-URL references", func() {
			s.NoError(validateChartReference("stable/grafana", cfg))
		})
	})
	s.Run("with allowed registries", func() {
		cfg := &Config{
			AllowedRegistries: []string{
				"oci://ghcr.io/myorg",
				"https://charts.example.com",
			},
		}
		s.Run("allows matching oci:// registry", func() {
			s.NoError(validateChartReference("oci://ghcr.io/myorg/mychart", cfg))
		})
		s.Run("allows matching https:// registry", func() {
			s.NoError(validateChartReference("https://charts.example.com/grafana-7.0.0.tgz", cfg))
		})
		s.Run("allows exact match of allowed entry", func() {
			s.NoError(validateChartReference("oci://ghcr.io/myorg", cfg))
		})
		s.Run("rejects similar prefix that is a different org", func() {
			err := validateChartReference("oci://ghcr.io/myorg-evil/chart", cfg)
			s.Error(err)
			s.Contains(err.Error(), "does not match any entry in allowed_registries")
		})
		s.Run("rejects path traversal to bypass allowlist", func() {
			err := validateChartReference("oci://ghcr.io/myorg/../evil-corp/chart", cfg)
			s.Error(err)
			s.Contains(err.Error(), "does not match any entry in allowed_registries")
		})
		s.Run("rejects non-matching oci:// registry", func() {
			err := validateChartReference("oci://ghcr.io/otherorg/chart", cfg)
			s.Error(err)
			s.Contains(err.Error(), "does not match any entry in allowed_registries")
		})
		s.Run("rejects non-matching https:// registry", func() {
			err := validateChartReference("https://evil.example.com/chart.tgz", cfg)
			s.Error(err)
			s.Contains(err.Error(), "does not match any entry in allowed_registries")
		})
		s.Run("rejects non-URL references", func() {
			err := validateChartReference("stable/grafana", cfg)
			s.Error(err)
			s.Contains(err.Error(), "only registry URLs from the allowed list are permitted")
		})
		s.Run("matches case-insensitively", func() {
			s.NoError(validateChartReference("OCI://GHCR.IO/myorg/mychart", cfg))
		})
		s.Run("matches registry with no path", func() {
			registryCfg := &Config{
				AllowedRegistries: []string{"oci://registry.example.com"},
			}
			s.NoError(validateChartReference("oci://registry.example.com/chart", registryCfg))
			s.NoError(validateChartReference("oci://registry.example.com", registryCfg))
		})
		s.Run("still blocks file:// scheme", func() {
			err := validateChartReference("file:///tmp/chart", cfg)
			s.Error(err)
			s.Contains(err.Error(), "file:// scheme is blocked")
		})
		s.Run("still blocks http:// scheme", func() {
			err := validateChartReference("http://evil.example.com/chart", cfg)
			s.Error(err)
			s.Contains(err.Error(), "http:// scheme is blocked")
		})
	})
}

func (s *HelmSuite) TestNewInstall() {
	s.Run("with nil config", func() {
		install := (&Helm{}).newInstall(&action.Configuration{})
		s.Run("waits for resource readiness", func() {
			s.True(install.Wait)
		})
		s.Run("uses the default timeout", func() {
			s.Equal(DefaultTimeout, install.Timeout)
		})
	})
	s.Run("with empty config", func() {
		install := (&Helm{config: &Config{}}).newInstall(&action.Configuration{})
		s.Run("waits for resource readiness", func() {
			s.True(install.Wait)
		})
		s.Run("uses the default timeout", func() {
			s.Equal(DefaultTimeout, install.Timeout)
		})
	})
	s.Run("applies configured wait = false", func() {
		install := (&Helm{config: &Config{Wait: ptr.To(false)}}).newInstall(&action.Configuration{})
		s.False(install.Wait)
	})
	s.Run("applies configured wait = true", func() {
		install := (&Helm{config: &Config{Wait: ptr.To(true)}}).newInstall(&action.Configuration{})
		s.True(install.Wait)
	})
	s.Run("applies configured timeout", func() {
		install := (&Helm{config: &Config{Timeout: "4m"}}).newInstall(&action.Configuration{})
		s.Equal(4*time.Minute, install.Timeout)
	})
	s.Run("falls back to the default timeout when malformed", func() {
		install := (&Helm{config: &Config{Timeout: "four minutes"}}).newInstall(&action.Configuration{})
		s.Equal(DefaultTimeout, install.Timeout)
	})
	s.Run("reads wait, not uninstall_wait", func() {
		// Disabling only uninstall waiting must leave install waiting.
		install := (&Helm{config: &Config{UninstallWait: ptr.To(false)}}).newInstall(&action.Configuration{})
		s.True(install.Wait)
	})
	s.Run("never dry runs", func() {
		install := (&Helm{config: &Config{}}).newInstall(&action.Configuration{})
		s.False(install.DryRun)
	})
}

func (s *HelmSuite) TestNewUninstall() {
	s.Run("with nil config", func() {
		uninstall := (&Helm{}).newUninstall(&action.Configuration{})
		s.Run("waits for resource deletion", func() {
			s.True(uninstall.Wait)
		})
		s.Run("uses the default timeout", func() {
			s.Equal(DefaultTimeout, uninstall.Timeout)
		})
	})
	s.Run("with empty config", func() {
		uninstall := (&Helm{config: &Config{}}).newUninstall(&action.Configuration{})
		s.Run("waits for resource deletion", func() {
			s.True(uninstall.Wait)
		})
		s.Run("uses the default timeout", func() {
			s.Equal(DefaultTimeout, uninstall.Timeout)
		})
	})
	s.Run("applies configured uninstall_wait = false", func() {
		uninstall := (&Helm{config: &Config{UninstallWait: ptr.To(false)}}).newUninstall(&action.Configuration{})
		s.False(uninstall.Wait)
	})
	s.Run("applies configured uninstall_wait = true", func() {
		uninstall := (&Helm{config: &Config{UninstallWait: ptr.To(true)}}).newUninstall(&action.Configuration{})
		s.True(uninstall.Wait)
	})
	s.Run("applies configured timeout", func() {
		uninstall := (&Helm{config: &Config{Timeout: "4m"}}).newUninstall(&action.Configuration{})
		s.Equal(4*time.Minute, uninstall.Timeout)
	})
	s.Run("reads uninstall_wait, not wait", func() {
		// Disabling install waiting must leave uninstall sequencing its deletes,
		// otherwise a caller that reinstalls immediately races the deletions.
		uninstall := (&Helm{config: &Config{Wait: ptr.To(false)}}).newUninstall(&action.Configuration{})
		s.True(uninstall.Wait)
	})
	s.Run("ignores a missing release", func() {
		uninstall := (&Helm{config: &Config{}}).newUninstall(&action.Configuration{})
		s.True(uninstall.IgnoreNotFound)
	})
}

func TestHelm(t *testing.T) {
	suite.Run(t, new(HelmSuite))
}
