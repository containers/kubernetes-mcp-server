package rulexpr

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type RuleExprSuite struct {
	suite.Suite
}

const eula = "verb in ['create', 'update', 'patch'] && kind == 'PipelineRun' && " +
	"has(object.spec.pipelineRef) && has(object.spec.pipelineRef.params) && object.spec.pipelineRef.params.exists(p, p.name == 'name' && p.value == 'windows-efi-installer') && " +
	"!(has(object.spec.params) && object.spec.params.exists(p, p.name == 'acceptEula' && p.value == 'true'))"

func (s *RuleExprSuite) TestCompile() {
	s.Run("rejects syntax errors", func() {
		_, err := Compile("verb ==")
		s.ErrorContains(err, "invalid expression")
	})
	s.Run("rejects non-boolean results", func() {
		_, err := Compile("verb")
		s.ErrorContains(err, "must evaluate to a boolean")
	})
	s.Run("accepts the EULA rule", func() {
		_, err := Compile(eula)
		s.NoError(err)
	})
}

func (s *RuleExprSuite) TestMatches() {
	windows := func(acceptEula string) map[string]any {
		params := []any{}
		if acceptEula != "" {
			params = append(params, map[string]any{"name": "acceptEula", "value": acceptEula})
		}
		return map[string]any{"spec": map[string]any{
			"pipelineRef": map[string]any{"resolver": "hub", "params": []any{map[string]any{"name": "name", "value": "windows-efi-installer"}}},
			"params":      params,
		}}
	}
	s.Run("matches a Windows PipelineRun without EULA acceptance", func() {
		matched, err := Matches(eula, Request{Verb: "create", Kind: "PipelineRun", Object: windows("")})
		s.Require().NoError(err)
		s.True(matched)
	})
	s.Run("does not match once the EULA is accepted", func() {
		matched, err := Matches(eula, Request{Verb: "create", Kind: "PipelineRun", Object: windows("true")})
		s.Require().NoError(err)
		s.False(matched)
	})
	s.Run("does not match other verbs, kinds or pipelines", func() {
		matched, _ := Matches(eula, Request{Verb: "delete", Kind: "PipelineRun", Object: windows("")})
		s.False(matched)
		matched, _ = Matches(eula, Request{Verb: "create", Kind: "TaskRun", Object: windows("")})
		s.False(matched)
	})
	s.Run("a missing object or field is not a match rather than an error", func() {
		matched, err := Matches("object.spec.replicas > 3", Request{Verb: "create", Kind: "Deployment"})
		s.Require().NoError(err)
		s.False(matched)
		matched, err = Matches("object.spec.replicas > 3", Request{Object: map[string]any{"spec": map[string]any{"replicas": 5.0}}})
		s.Require().NoError(err)
		s.True(matched)
	})
}

func TestRuleExpr(t *testing.T) {
	suite.Run(t, new(RuleExprSuite))
}
