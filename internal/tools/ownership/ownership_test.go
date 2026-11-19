package ownership_test

import (
	"path/filepath"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/tools/ownership"
	"github.com/stretchr/testify/suite"
)

type OwnershipSuite struct{ suite.Suite }

func TestOwnership(t *testing.T) {
	suite.Run(t, new(OwnershipSuite))
}

func (s *OwnershipSuite) TestLoad() {
	s.Run("accepts valid roles", func() {
		config, err := ownership.Load(filepath.Join("testdata", "valid.json"))
		s.Require().NoError(err)
		s.Equal("@mjudeikis", config.Areas[0].Reviewers[0])
	})
	s.Run("rejects invalid files", func() {
		for _, file := range []string{"unknown-field.json", "trailing-document.json", "missing-paths.json"} {
			s.Run(file, func() {
				_, err := ownership.Load(filepath.Join("testdata", file))
				s.Error(err)
			})
		}
	})
}

func (s *OwnershipSuite) TestValidate() {
	invalid := func(name string, change func(*ownership.Config)) {
		s.Run(name, func() {
			config := ownership.Config{
				DefaultApprovers: []string{"@Cali0707"},
				Areas:            []ownership.Area{{Toolset: "logs", Paths: []string{"/docs/KIALI.md"}}},
			}
			change(&config)
			s.Error(config.Validate())
		})
	}
	invalid("empty default approvers", func(c *ownership.Config) { c.DefaultApprovers = nil })
	invalid("invalid default user", func(c *ownership.Config) { c.DefaultApprovers = []string{"not-a-user"} })
	invalid("invalid default team", func(c *ownership.Config) { c.DefaultReviewers = []string{"@org/bad team"} })
	invalid("missing paths", func(c *ownership.Config) { c.Areas[0].Paths = nil })
	invalid("invalid toolset", func(c *ownership.Config) { c.Areas[0].Toolset = "observability//logs" })
	invalid("duplicate toolset", func(c *ownership.Config) {
		c.Areas = append(c.Areas, ownership.Area{Toolset: "logs", Paths: []string{"/other/"}})
	})
	invalid("invalid area approver", func(c *ownership.Config) { c.Areas[0].Approvers = []string{"bad"} })
	invalid("invalid area reviewer", func(c *ownership.Config) { c.Areas[0].Reviewers = []string{"bad"} })
	invalid("relative path", func(c *ownership.Config) { c.Areas[0].Paths = []string{"pkg/logs/"} })
	invalid("bad wildcard", func(c *ownership.Config) { c.Areas[0].Paths = []string{"/pkg/*/logs/"} })
	invalid("space in path", func(c *ownership.Config) { c.Areas[0].Paths = []string{"/pkg/bad path/"} })
	invalid("nested directory", func(c *ownership.Config) {
		c.Areas = append(c.Areas, ownership.Area{Toolset: "docs", Paths: []string{"/docs/"}})
	})
	invalid("nested wildcard", func(c *ownership.Config) {
		c.Areas = append(c.Areas, ownership.Area{Toolset: "docs", Paths: []string{"/docs/KI*"}})
	})
	invalid("nested in same area", func(c *ownership.Config) { c.Areas[0].Paths = append(c.Areas[0].Paths, "/docs/") })
	s.Run("allows identical shared paths", func() {
		config := ownership.Config{
			DefaultApprovers: []string{"@Cali0707"},
			Areas: []ownership.Area{
				{Toolset: "logs", Paths: []string{"/pkg/obs/auth/"}},
				{Toolset: "metrics", Paths: []string{"/pkg/obs/auth/"}},
			},
		}
		s.NoError(config.Validate())
	})
	s.Run("does not confuse sibling directories", func() {
		config := ownership.Config{
			DefaultApprovers: []string{"@Cali0707"},
			Areas: []ownership.Area{
				{Toolset: "logs", Paths: []string{"/docs/logs/"}},
				{Toolset: "metrics", Paths: []string{"/docs/logs-extra/"}},
			},
		}
		s.NoError(config.Validate())
	})
}
