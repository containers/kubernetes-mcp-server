package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/tools/ownership"
	"github.com/stretchr/testify/suite"
)

type CodeownersSuite struct{ suite.Suite }

func TestCodeowners(t *testing.T) {
	suite.Run(t, new(CodeownersSuite))
}

func (s *CodeownersSuite) TestApproversNotReviewers() {
	result := s.renderFixture("roles.json")
	s.Run("default and area approvers", func() {
		for _, line := range []string{
			"* @Cali0707 @manusa",
			"/pkg/toolsets/kiali/ @Cali0707 @manusa @matzew",
			"/pkg/kiali/ @Cali0707 @manusa @matzew",
		} {
			s.Run(line, func() { s.True(hasRule(result, line)) })
		}
	})
	s.Run("toolset section", func() { s.Contains(result, "# toolset: kiali\n") })
	s.Run("reviewers are excluded", func() {
		s.NotContains(result, "@aljesusg")
		s.NotContains(result, "@mjudeikis")
	})
}

func (s *CodeownersSuite) TestSharedPathsCombineApprovers() {
	result := s.renderFixture("shared.json")
	s.Run("all approvers are retained", func() {
		s.True(hasRule(result, "/pkg/obs/toolcfg/ @Cali0707 @manusa @matzew @nader-ziada @2uasimojo"))
	})
	s.Run("shared path emitted once", func() {
		s.Equal(1, strings.Count(result, "/pkg/obs/toolcfg/"))
	})
	s.Run("distinct shared groups", func() {
		s.Contains(result, "# shared: logs, metrics, traces\n")
		s.Contains(result, "# shared: logs, metrics, otelcol, traces\n")
	})
}

func (s *CodeownersSuite) TestRolesStayScopedToAreas() {
	result := s.renderFixture("scoped.json")
	s.Run("approvers belong to their area", func() {
		s.True(hasRule(result, "/pkg/toolsets/helm/ @Cali0707 @manusa"))
		s.True(hasRule(result, "/pkg/toolsets/kcp/ @Cali0707"))
	})
	s.Run("reviewers are excluded", func() { s.NotContains(result, "@mjudeikis") })
}

func (s *CodeownersSuite) renderFixture(file string) string {
	config, err := ownership.Load(filepath.Join("testdata", file))
	s.Require().NoError(err)
	result, err := render(config)
	s.Require().NoError(err)
	return string(result)
}

func hasRule(content, expected string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.Join(strings.Fields(line), " ") == expected {
			return true
		}
	}
	return false
}
