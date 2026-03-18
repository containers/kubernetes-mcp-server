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
		matched := MatchToolLevelRules(rules, "helm_uninstall", nil, nil)
		s.Require().Len(matched, 1)
		s.Equal("uninstall", matched[0].Message)
	})
	s.Run("does not match different tool name", func() {
		rules := []api.ConfirmationRule{
			{Tool: "helm_uninstall", Message: "uninstall"},
		}
		matched := MatchToolLevelRules(rules, "pods_list", nil, nil)
		s.Empty(matched)
	})
	s.Run("matches by tool name and input", func() {
		rules := []api.ConfirmationRule{
			{Tool: "resources_delete", Input: map[string]any{"namespace": "kube-system"}, Message: "delete in kube-system"},
		}
		args := map[string]any{"namespace": "kube-system", "name": "my-pod"}
		matched := MatchToolLevelRules(rules, "resources_delete", args, nil)
		s.Require().Len(matched, 1)
	})
	s.Run("does not match when input value differs", func() {
		rules := []api.ConfirmationRule{
			{Tool: "resources_delete", Input: map[string]any{"namespace": "kube-system"}, Message: "msg"},
		}
		args := map[string]any{"namespace": "default"}
		matched := MatchToolLevelRules(rules, "resources_delete", args, nil)
		s.Empty(matched)
	})
	s.Run("does not match when input key is missing from args", func() {
		rules := []api.ConfirmationRule{
			{Tool: "resources_delete", Input: map[string]any{"namespace": "kube-system"}, Message: "msg"},
		}
		args := map[string]any{"name": "my-pod"}
		matched := MatchToolLevelRules(rules, "resources_delete", args, nil)
		s.Empty(matched)
	})
	s.Run("matches numeric input values", func() {
		rules := []api.ConfirmationRule{
			{Tool: "scale", Input: map[string]any{"replicas": float64(0)}, Message: "scaling to zero"},
		}
		args := map[string]any{"replicas": float64(0)}
		matched := MatchToolLevelRules(rules, "scale", args, nil)
		s.Require().Len(matched, 1)
	})
	s.Run("matches boolean input values", func() {
		rules := []api.ConfirmationRule{
			{Tool: "apply", Input: map[string]any{"force": true}, Message: "force apply"},
		}
		args := map[string]any{"force": true}
		matched := MatchToolLevelRules(rules, "apply", args, nil)
		s.Require().Len(matched, 1)
	})
	s.Run("matches by destructive hint true", func() {
		rules := []api.ConfirmationRule{
			{Destructive: ptr.To(true), Message: "destructive"},
		}
		matched := MatchToolLevelRules(rules, "any_tool", nil, ptr.To(true))
		s.Require().Len(matched, 1)
	})
	s.Run("does not match destructive rule when hint is false", func() {
		rules := []api.ConfirmationRule{
			{Destructive: ptr.To(true), Message: "destructive"},
		}
		matched := MatchToolLevelRules(rules, "any_tool", nil, ptr.To(false))
		s.Empty(matched)
	})
	s.Run("does not match destructive rule when hint is nil", func() {
		rules := []api.ConfirmationRule{
			{Destructive: ptr.To(true), Message: "destructive"},
		}
		matched := MatchToolLevelRules(rules, "any_tool", nil, nil)
		s.Empty(matched)
	})
	s.Run("skips kube-level rules", func() {
		rules := []api.ConfirmationRule{
			{Verb: "delete", Message: "kube rule"},
		}
		matched := MatchToolLevelRules(rules, "any_tool", nil, nil)
		s.Empty(matched)
	})
	s.Run("returns multiple matches", func() {
		rules := []api.ConfirmationRule{
			{Tool: "helm_uninstall", Message: "tool match"},
			{Destructive: ptr.To(true), Message: "destructive match"},
		}
		matched := MatchToolLevelRules(rules, "helm_uninstall", nil, ptr.To(true))
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
			{Message: "Deleting resource.", Fallback: "deny"},
		}
		message, fallback := MergeMatchedRules(matched, "allow")
		s.Equal("Deleting resource.", message)
		s.Equal("deny", fallback)
	})
	s.Run("single rule uses global fallback when unset", func() {
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
	s.Run("deny wins over allow in mixed fallbacks", func() {
		matched := []api.ConfirmationRule{
			{Message: "msg1", Fallback: "allow"},
			{Message: "msg2", Fallback: "deny"},
		}
		_, fallback := MergeMatchedRules(matched, "allow")
		s.Equal("deny", fallback)
	})
	s.Run("all allow produces allow fallback", func() {
		matched := []api.ConfirmationRule{
			{Message: "msg1", Fallback: "allow"},
			{Message: "msg2", Fallback: "allow"},
		}
		_, fallback := MergeMatchedRules(matched, "allow")
		s.Equal("allow", fallback)
	})
	s.Run("global deny wins when rules have no override", func() {
		matched := []api.ConfirmationRule{
			{Message: "msg1"},
			{Message: "msg2"},
		}
		_, fallback := MergeMatchedRules(matched, "deny")
		s.Equal("deny", fallback)
	})
}

func (s *MatchSuite) TestNormalizeInput() {
	s.Run("converts int64 to float64", func() {
		input := map[string]any{"replicas": int64(3)}
		result := NormalizeInput(input)
		s.Equal(float64(3), result["replicas"])
	})
	s.Run("leaves float64 unchanged", func() {
		input := map[string]any{"ratio": float64(1.5)}
		result := NormalizeInput(input)
		s.Equal(float64(1.5), result["ratio"])
	})
	s.Run("leaves strings unchanged", func() {
		input := map[string]any{"namespace": "kube-system"}
		result := NormalizeInput(input)
		s.Equal("kube-system", result["namespace"])
	})
	s.Run("leaves booleans unchanged", func() {
		input := map[string]any{"force": true}
		result := NormalizeInput(input)
		s.Equal(true, result["force"])
	})
	s.Run("handles nil input", func() {
		result := NormalizeInput(nil)
		s.Nil(result)
	})
}

func TestMatch(t *testing.T) {
	suite.Run(t, new(MatchSuite))
}
