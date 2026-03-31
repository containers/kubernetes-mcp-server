package confirmation

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
	"k8s.io/utils/ptr"
)

type MatchSuite struct {
	suite.Suite
}

func (s *MatchSuite) TestMatchToolLevelRules() {
	s.Run("matches by tool name", func() {
		rules := []api.ConfirmationRule{
			{Tool: "helm_uninstall", Message: "uninstall"},
		}
		matched := MatchToolLevelRules(rules, "helm_uninstall", nil)
		s.Require().Len(matched, 1)
		s.Equal("uninstall", matched[0].Message)
	})
	s.Run("does not match different tool name", func() {
		rules := []api.ConfirmationRule{
			{Tool: "helm_uninstall", Message: "uninstall"},
		}
		matched := MatchToolLevelRules(rules, "pods_list", nil)
		s.Empty(matched)
	})
	s.Run("matches by destructive hint true", func() {
		rules := []api.ConfirmationRule{
			{Destructive: ptr.To(true), Message: "destructive"},
		}
		matched := MatchToolLevelRules(rules, "any_tool", ptr.To(true))
		s.Require().Len(matched, 1)
	})
	s.Run("does not match destructive rule when hint is false", func() {
		rules := []api.ConfirmationRule{
			{Destructive: ptr.To(true), Message: "destructive"},
		}
		matched := MatchToolLevelRules(rules, "any_tool", ptr.To(false))
		s.Empty(matched)
	})
	s.Run("does not match destructive rule when hint is nil", func() {
		rules := []api.ConfirmationRule{
			{Destructive: ptr.To(true), Message: "destructive"},
		}
		matched := MatchToolLevelRules(rules, "any_tool", nil)
		s.Empty(matched)
	})
	s.Run("skips kube-level rules", func() {
		rules := []api.ConfirmationRule{
			{Verb: "delete", Message: "kube rule"},
		}
		matched := MatchToolLevelRules(rules, "any_tool", nil)
		s.Empty(matched)
	})
	s.Run("returns multiple matches", func() {
		rules := []api.ConfirmationRule{
			{Tool: "helm_uninstall", Message: "tool match"},
			{Destructive: ptr.To(true), Message: "destructive match"},
		}
		matched := MatchToolLevelRules(rules, "helm_uninstall", ptr.To(true))
		s.Len(matched, 2)
	})
}

func (s *MatchSuite) TestMatchKubeLevelRules() {
	s.Run("matches by verb", func() {
		rules := []api.ConfirmationRule{
			{Verb: "delete", Message: "deleting"},
		}
		matched := MatchKubeLevelRules(rules, "delete", "Pod", "", "v1", "", "default")
		s.Require().Len(matched, 1)
	})
	s.Run("matches by verb and kind", func() {
		rules := []api.ConfirmationRule{
			{Verb: "get", Kind: "Secret", Message: "accessing secret"},
		}
		matched := MatchKubeLevelRules(rules, "get", "Secret", "", "v1", "", "default")
		s.Require().Len(matched, 1)
	})
	s.Run("matches by verb and namespace", func() {
		rules := []api.ConfirmationRule{
			{Verb: "delete", Namespace: "kube-system", Message: "delete in kube-system"},
		}
		matched := MatchKubeLevelRules(rules, "delete", "Pod", "", "v1", "", "kube-system")
		s.Require().Len(matched, 1)
	})
	s.Run("matches by full GVK", func() {
		rules := []api.ConfirmationRule{
			{Verb: "delete", Kind: "Deployment", Group: "apps", Version: "v1", Message: "delete deployment"},
		}
		matched := MatchKubeLevelRules(rules, "delete", "Deployment", "apps", "v1", "", "default")
		s.Require().Len(matched, 1)
	})
	s.Run("does not match different verb", func() {
		rules := []api.ConfirmationRule{
			{Verb: "delete", Message: "deleting"},
		}
		matched := MatchKubeLevelRules(rules, "get", "Pod", "", "v1", "", "default")
		s.Empty(matched)
	})
	s.Run("does not match different kind", func() {
		rules := []api.ConfirmationRule{
			{Verb: "get", Kind: "Secret", Message: "accessing secret"},
		}
		matched := MatchKubeLevelRules(rules, "get", "ConfigMap", "", "v1", "", "default")
		s.Empty(matched)
	})
	s.Run("does not match different namespace", func() {
		rules := []api.ConfirmationRule{
			{Verb: "delete", Namespace: "kube-system", Message: "msg"},
		}
		matched := MatchKubeLevelRules(rules, "delete", "Pod", "", "v1", "", "default")
		s.Empty(matched)
	})
	s.Run("skips tool-level rules", func() {
		rules := []api.ConfirmationRule{
			{Tool: "helm_uninstall", Message: "tool rule"},
		}
		matched := MatchKubeLevelRules(rules, "delete", "Pod", "", "v1", "", "default")
		s.Empty(matched)
	})
	s.Run("matches by name", func() {
		rules := []api.ConfirmationRule{
			{Verb: "get", Kind: "Secret", Name: "my-secret", Message: "accessing specific secret"},
		}
		matched := MatchKubeLevelRules(rules, "get", "Secret", "", "v1", "my-secret", "default")
		s.Require().Len(matched, 1)
	})
	s.Run("does not match different name", func() {
		rules := []api.ConfirmationRule{
			{Verb: "get", Kind: "Secret", Name: "my-secret", Message: "accessing specific secret"},
		}
		matched := MatchKubeLevelRules(rules, "get", "Secret", "", "v1", "other-secret", "default")
		s.Empty(matched)
	})
	s.Run("matches by name and namespace", func() {
		rules := []api.ConfirmationRule{
			{Verb: "get", Kind: "Secret", Name: "my-secret", Namespace: "production", Message: "accessing secret in prod"},
		}
		matched := MatchKubeLevelRules(rules, "get", "Secret", "", "v1", "my-secret", "production")
		s.Require().Len(matched, 1)
	})
	s.Run("does not match name when namespace differs", func() {
		rules := []api.ConfirmationRule{
			{Verb: "get", Kind: "Secret", Name: "my-secret", Namespace: "production", Message: "msg"},
		}
		matched := MatchKubeLevelRules(rules, "get", "Secret", "", "v1", "my-secret", "staging")
		s.Empty(matched)
	})
	s.Run("name-only rule makes it kube-level", func() {
		rules := []api.ConfirmationRule{
			{Name: "critical-resource", Message: "touching critical resource"},
		}
		s.True(rules[0].IsKubeLevel())
		matched := MatchKubeLevelRules(rules, "delete", "Pod", "", "v1", "critical-resource", "default")
		s.Require().Len(matched, 1)
	})
	s.Run("namespace-only rule makes it kube-level", func() {
		rules := []api.ConfirmationRule{
			{Namespace: "kube-system", Message: "operating in kube-system"},
		}
		s.True(rules[0].IsKubeLevel())
		matched := MatchKubeLevelRules(rules, "delete", "Pod", "", "v1", "", "kube-system")
		s.Require().Len(matched, 1)
	})
	s.Run("returns multiple matches", func() {
		rules := []api.ConfirmationRule{
			{Verb: "delete", Message: "delete rule"},
			{Verb: "delete", Namespace: "kube-system", Message: "kube-system rule"},
		}
		matched := MatchKubeLevelRules(rules, "delete", "Pod", "", "v1", "", "kube-system")
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
		matched := []api.ConfirmationRule{
			{Message: "Deleting resource."},
		}
		message, fallback := MergeMatchedRules(matched, "deny")
		s.Equal("Deleting resource.", message)
		s.Equal("deny", fallback)
	})
	s.Run("single rule uses global fallback", func() {
		matched := []api.ConfirmationRule{
			{Message: "Deleting resource."},
		}
		_, fallback := MergeMatchedRules(matched, "allow")
		s.Equal("allow", fallback)
	})
	s.Run("multiple rules produce bulleted message", func() {
		matched := []api.ConfirmationRule{
			{Message: "Destructive operation."},
			{Message: "Uninstalling Helm release."},
		}
		message, _ := MergeMatchedRules(matched, "allow")
		s.Contains(message, "Confirmation required:")
		s.Contains(message, "- Destructive operation.")
		s.Contains(message, "- Uninstalling Helm release.")
	})
	s.Run("multiple rules use global fallback", func() {
		matched := []api.ConfirmationRule{
			{Message: "msg1"},
			{Message: "msg2"},
		}
		_, fallback := MergeMatchedRules(matched, "deny")
		s.Equal("deny", fallback)
	})
}

func (s *MatchSuite) TestConfirmationRuleValidation() {
	s.Run("valid tool-level rule", func() {
		rule := api.ConfirmationRule{Tool: "helm_uninstall", Message: "uninstall"}
		s.NoError(rule.Validate())
	})
	s.Run("valid destructive rule", func() {
		rule := api.ConfirmationRule{Destructive: ptr.To(true), Message: "destructive"}
		s.NoError(rule.Validate())
	})
	s.Run("valid kube-level rule", func() {
		rule := api.ConfirmationRule{Verb: "delete", Kind: "Pod", Message: "delete pod"}
		s.NoError(rule.Validate())
	})
	s.Run("valid namespace-only kube rule", func() {
		rule := api.ConfirmationRule{Namespace: "kube-system", Message: "kube-system"}
		s.NoError(rule.Validate())
	})
	s.Run("valid name-only kube rule", func() {
		rule := api.ConfirmationRule{Name: "critical-resource", Message: "critical"}
		s.NoError(rule.Validate())
	})
	s.Run("rejects mixed tool and kube fields", func() {
		rule := api.ConfirmationRule{Tool: "resources_delete", Verb: "delete", Message: "mixed"}
		s.Error(rule.Validate())
	})
	s.Run("rejects tool with namespace", func() {
		rule := api.ConfirmationRule{Tool: "resources_delete", Namespace: "kube-system", Message: "mixed"}
		s.Error(rule.Validate())
	})
	s.Run("rejects rule with no level fields", func() {
		rule := api.ConfirmationRule{Message: "orphan rule"}
		s.Error(rule.Validate())
	})
}

func TestMatch(t *testing.T) {
	suite.Run(t, new(MatchSuite))
}
