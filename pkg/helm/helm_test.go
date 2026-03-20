package helm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	helmtime "helm.sh/helm/v3/pkg/time"
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

func TestHelm(t *testing.T) {
	suite.Run(t, new(HelmSuite))
}
