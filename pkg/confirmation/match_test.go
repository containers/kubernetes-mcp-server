package confirmation

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/confirmation/rulexpr"
	"github.com/stretchr/testify/suite"
	"k8s.io/utils/ptr"
)

type MatchSuite struct {
	suite.Suite
}

func (s *MatchSuite) TestMatchToolLevelRules() {
	s.Run("matches by tool name", func() {
		rules := []config.ConfirmationRule{
			{Tool: "helm_uninstall", Message: "uninstall"},
		}
		matched := MatchToolLevelRules(rules, "helm_uninstall", nil)
		s.Require().Len(matched, 1)
		s.Equal("uninstall", matched[0].Message)
	})
	s.Run("does not match different tool name", func() {
		rules := []config.ConfirmationRule{
			{Tool: "helm_uninstall", Message: "uninstall"},
		}
		matched := MatchToolLevelRules(rules, "pods_list", nil)
		s.Empty(matched)
	})
	s.Run("matches by destructive hint true", func() {
		rules := []config.ConfirmationRule{
			{Destructive: ptr.To(true), Message: "destructive"},
		}
		matched := MatchToolLevelRules(rules, "any_tool", ptr.To(true))
		s.Require().Len(matched, 1)
	})
	s.Run("does not match destructive rule when hint is false", func() {
		rules := []config.ConfirmationRule{
			{Destructive: ptr.To(true), Message: "destructive"},
		}
		matched := MatchToolLevelRules(rules, "any_tool", ptr.To(false))
		s.Empty(matched)
	})
	s.Run("does not match destructive rule when hint is nil", func() {
		rules := []config.ConfirmationRule{
			{Destructive: ptr.To(true), Message: "destructive"},
		}
		matched := MatchToolLevelRules(rules, "any_tool", nil)
		s.Empty(matched)
	})
	s.Run("skips kube-level rules", func() {
		rules := []config.ConfirmationRule{
			{Verb: "delete", Message: "kube rule"},
		}
		matched := MatchToolLevelRules(rules, "any_tool", nil)
		s.Empty(matched)
	})
	s.Run("returns multiple matches", func() {
		rules := []config.ConfirmationRule{
			{Tool: "helm_uninstall", Message: "tool match"},
			{Destructive: ptr.To(true), Message: "destructive match"},
		}
		matched := MatchToolLevelRules(rules, "helm_uninstall", ptr.To(true))
		s.Len(matched, 2)
	})
}

func (s *MatchSuite) TestMatchKubeLevelRequest() {
	ctx := s.T().Context()
	windows := map[string]any{"spec": map[string]any{"pipelineRef": map[string]any{"params": []any{map[string]any{"name": "name", "value": "windows-efi-installer"}}}}}
	s.Run("a true expression matches", func() {
		rules := []config.ConfirmationRule{{Kind: "PipelineRun", Expression: "object.spec.pipelineRef.params.exists(p, p.value == 'windows-efi-installer')", Message: "EULA"}}
		matched := MatchKubeLevelRequest(ctx, rules, rulexpr.Request{Verb: "create", Kind: "PipelineRun", Object: windows})
		s.Require().Len(matched, 1)
		s.Equal("EULA", matched[0].Message)
	})
	s.Run("a false expression does not match", func() {
		rules := []config.ConfirmationRule{{Kind: "PipelineRun", Expression: "object.spec.pipelineRef.params.exists(p, p.value == 'other')", Message: "EULA"}}
		s.Empty(MatchKubeLevelRequest(ctx, rules, rulexpr.Request{Verb: "create", Kind: "PipelineRun", Object: windows}))
	})
	s.Run("an expression that cannot be evaluated fails closed", func() {
		// A typo outside a has() guard, or a body the rule did not expect, must
		// require confirmation rather than silently disable the rule.
		rules := []config.ConfirmationRule{{Kind: "PipelineRun", Expression: "object.spec.pipelinRef.params.size() > 0", Message: "EULA"}}
		matched := MatchKubeLevelRequest(ctx, rules, rulexpr.Request{Verb: "create", Kind: "PipelineRun", Object: windows})
		s.Require().Len(matched, 1)
		s.Contains(matched[0].Message, "EULA")
		s.Contains(matched[0].Message, "could not be evaluated")
		s.Contains(matched[0].Message, "pipelinRef")
	})
	s.Run("the other fields still have to match before the expression counts", func() {
		rules := []config.ConfirmationRule{{Kind: "TaskRun", Expression: "object.spec.pipelinRef.params.size() > 0", Message: "EULA"}}
		s.Empty(MatchKubeLevelRequest(ctx, rules, rulexpr.Request{Verb: "create", Kind: "PipelineRun", Object: windows}))
	})
}

func (s *MatchSuite) TestMatchKubeLevelRequestByField() {
	s.Run("matches by verb", func() {
		rules := []config.ConfirmationRule{
			{Verb: "delete", Message: "deleting"},
		}
		matched := MatchKubeLevelRequest(s.T().Context(), rules, rulexpr.Request{Verb: "delete", Kind: "Pod", Version: "v1", Namespace: "default"})
		s.Require().Len(matched, 1)
	})
	s.Run("matches by verb and kind", func() {
		rules := []config.ConfirmationRule{
			{Verb: "get", Kind: "Secret", Message: "accessing secret"},
		}
		matched := MatchKubeLevelRequest(s.T().Context(), rules, rulexpr.Request{Verb: "get", Kind: "Secret", Version: "v1", Namespace: "default"})
		s.Require().Len(matched, 1)
	})
	s.Run("matches by verb and namespace", func() {
		rules := []config.ConfirmationRule{
			{Verb: "delete", Namespace: "kube-system", Message: "delete in kube-system"},
		}
		matched := MatchKubeLevelRequest(s.T().Context(), rules, rulexpr.Request{Verb: "delete", Kind: "Pod", Version: "v1", Namespace: "kube-system"})
		s.Require().Len(matched, 1)
	})
	s.Run("matches by full GVK", func() {
		rules := []config.ConfirmationRule{
			{Verb: "delete", Kind: "Deployment", Group: "apps", Version: "v1", Message: "delete deployment"},
		}
		matched := MatchKubeLevelRequest(s.T().Context(), rules, rulexpr.Request{Verb: "delete", Kind: "Deployment", Group: "apps", Version: "v1", Namespace: "default"})
		s.Require().Len(matched, 1)
	})
	s.Run("does not match different verb", func() {
		rules := []config.ConfirmationRule{
			{Verb: "delete", Message: "deleting"},
		}
		matched := MatchKubeLevelRequest(s.T().Context(), rules, rulexpr.Request{Verb: "get", Kind: "Pod", Version: "v1", Namespace: "default"})
		s.Empty(matched)
	})
	s.Run("does not match different kind", func() {
		rules := []config.ConfirmationRule{
			{Verb: "get", Kind: "Secret", Message: "accessing secret"},
		}
		matched := MatchKubeLevelRequest(s.T().Context(), rules, rulexpr.Request{Verb: "get", Kind: "ConfigMap", Version: "v1", Namespace: "default"})
		s.Empty(matched)
	})
	s.Run("does not match different namespace", func() {
		rules := []config.ConfirmationRule{
			{Verb: "delete", Namespace: "kube-system", Message: "msg"},
		}
		matched := MatchKubeLevelRequest(s.T().Context(), rules, rulexpr.Request{Verb: "delete", Kind: "Pod", Version: "v1", Namespace: "default"})
		s.Empty(matched)
	})
	s.Run("skips tool-level rules", func() {
		rules := []config.ConfirmationRule{
			{Tool: "helm_uninstall", Message: "tool rule"},
		}
		matched := MatchKubeLevelRequest(s.T().Context(), rules, rulexpr.Request{Verb: "delete", Kind: "Pod", Version: "v1", Namespace: "default"})
		s.Empty(matched)
	})
	s.Run("matches by name", func() {
		rules := []config.ConfirmationRule{
			{Verb: "get", Kind: "Secret", Name: "my-secret", Message: "accessing specific secret"},
		}
		matched := MatchKubeLevelRequest(s.T().Context(), rules, rulexpr.Request{Verb: "get", Kind: "Secret", Version: "v1", Name: "my-secret", Namespace: "default"})
		s.Require().Len(matched, 1)
	})
	s.Run("does not match different name", func() {
		rules := []config.ConfirmationRule{
			{Verb: "get", Kind: "Secret", Name: "my-secret", Message: "accessing specific secret"},
		}
		matched := MatchKubeLevelRequest(s.T().Context(), rules, rulexpr.Request{Verb: "get", Kind: "Secret", Version: "v1", Name: "other-secret", Namespace: "default"})
		s.Empty(matched)
	})
	s.Run("matches by name and namespace", func() {
		rules := []config.ConfirmationRule{
			{Verb: "get", Kind: "Secret", Name: "my-secret", Namespace: "production", Message: "accessing secret in prod"},
		}
		matched := MatchKubeLevelRequest(s.T().Context(), rules, rulexpr.Request{Verb: "get", Kind: "Secret", Version: "v1", Name: "my-secret", Namespace: "production"})
		s.Require().Len(matched, 1)
	})
	s.Run("does not match name when namespace differs", func() {
		rules := []config.ConfirmationRule{
			{Verb: "get", Kind: "Secret", Name: "my-secret", Namespace: "production", Message: "msg"},
		}
		matched := MatchKubeLevelRequest(s.T().Context(), rules, rulexpr.Request{Verb: "get", Kind: "Secret", Version: "v1", Name: "my-secret", Namespace: "staging"})
		s.Empty(matched)
	})
	s.Run("name-only rule makes it kube-level", func() {
		rules := []config.ConfirmationRule{
			{Name: "critical-resource", Message: "touching critical resource"},
		}
		s.True(rules[0].IsKubeLevel())
		matched := MatchKubeLevelRequest(s.T().Context(), rules, rulexpr.Request{Verb: "delete", Kind: "Pod", Version: "v1", Name: "critical-resource", Namespace: "default"})
		s.Require().Len(matched, 1)
	})
	s.Run("namespace-only rule makes it kube-level", func() {
		rules := []config.ConfirmationRule{
			{Namespace: "kube-system", Message: "operating in kube-system"},
		}
		s.True(rules[0].IsKubeLevel())
		matched := MatchKubeLevelRequest(s.T().Context(), rules, rulexpr.Request{Verb: "delete", Kind: "Pod", Version: "v1", Namespace: "kube-system"})
		s.Require().Len(matched, 1)
	})
	s.Run("returns multiple matches", func() {
		rules := []config.ConfirmationRule{
			{Verb: "delete", Message: "delete rule"},
			{Verb: "delete", Namespace: "kube-system", Message: "kube-system rule"},
		}
		matched := MatchKubeLevelRequest(s.T().Context(), rules, rulexpr.Request{Verb: "delete", Kind: "Pod", Version: "v1", Namespace: "kube-system"})
		s.Len(matched, 2)
	})
}

func (s *MatchSuite) TestMergeMatchedRules() {
	s.Run("empty matched returns empty message", func() {
		message, fallback := MergeMatchedRules(nil, "allow")
		s.Empty(message)
		s.Equal("allow", fallback)
	})
	s.Run("single rule passes message through", func() {
		matched := []config.ConfirmationRule{
			{Message: "Deleting resource."},
		}
		message, fallback := MergeMatchedRules(matched, "deny")
		s.Equal("Deleting resource.", message)
		s.Equal("deny", fallback)
	})
	s.Run("single rule uses global fallback", func() {
		matched := []config.ConfirmationRule{
			{Message: "Deleting resource."},
		}
		_, fallback := MergeMatchedRules(matched, "allow")
		s.Equal("allow", fallback)
	})
	s.Run("multiple rules produce bulleted message", func() {
		matched := []config.ConfirmationRule{
			{Message: "Destructive operation."},
			{Message: "Uninstalling Helm release."},
		}
		message, _ := MergeMatchedRules(matched, "allow")
		s.Contains(message, "Confirmation required:")
		s.Contains(message, "- Destructive operation.")
		s.Contains(message, "- Uninstalling Helm release.")
	})
	s.Run("multiple rules use global fallback", func() {
		matched := []config.ConfirmationRule{
			{Message: "msg1"},
			{Message: "msg2"},
		}
		_, fallback := MergeMatchedRules(matched, "deny")
		s.Equal("deny", fallback)
	})
}

func (s *MatchSuite) TestConfirmationRuleValidation() {
	s.Run("valid tool-level rule", func() {
		rule := config.ConfirmationRule{Tool: "helm_uninstall", Message: "uninstall"}
		s.NoError(rule.Validate())
	})
	s.Run("valid destructive rule", func() {
		rule := config.ConfirmationRule{Destructive: ptr.To(true), Message: "destructive"}
		s.NoError(rule.Validate())
	})
	s.Run("valid kube-level rule", func() {
		rule := config.ConfirmationRule{Verb: "delete", Kind: "Pod", Message: "delete pod"}
		s.NoError(rule.Validate())
	})
	s.Run("valid namespace-only kube rule", func() {
		rule := config.ConfirmationRule{Namespace: "kube-system", Message: "kube-system"}
		s.NoError(rule.Validate())
	})
	s.Run("valid name-only kube rule", func() {
		rule := config.ConfirmationRule{Name: "critical-resource", Message: "critical"}
		s.NoError(rule.Validate())
	})
	s.Run("rejects mixed tool and kube fields", func() {
		rule := config.ConfirmationRule{Tool: "resources_delete", Verb: "delete", Message: "mixed"}
		s.Error(rule.Validate())
	})
	s.Run("rejects tool with namespace", func() {
		rule := config.ConfirmationRule{Tool: "resources_delete", Namespace: "kube-system", Message: "mixed"}
		s.Error(rule.Validate())
	})
	s.Run("rejects rule with no level fields", func() {
		rule := config.ConfirmationRule{Message: "orphan rule"}
		s.Error(rule.Validate())
	})
}

func TestMatch(t *testing.T) {
	suite.Run(t, new(MatchSuite))
}
