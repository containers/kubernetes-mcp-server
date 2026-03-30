package api

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"k8s.io/utils/ptr"
)

type ConfirmationRuleSuite struct {
	suite.Suite
}

func (s *ConfirmationRuleSuite) TestIsToolLevel() {
	s.Run("true when tool is set", func() {
		r := ConfirmationRule{Tool: "helm_uninstall"}
		s.True(r.IsToolLevel())
	})
	s.Run("true when destructive is set", func() {
		r := ConfirmationRule{Destructive: ptr.To(true)}
		s.True(r.IsToolLevel())
	})
	s.Run("true when destructive is false", func() {
		r := ConfirmationRule{Destructive: ptr.To(false)}
		s.True(r.IsToolLevel())
	})
	s.Run("false when only kube fields are set", func() {
		r := ConfirmationRule{Verb: "delete", Kind: "Pod"}
		s.False(r.IsToolLevel())
	})
	s.Run("false when no fields are set", func() {
		r := ConfirmationRule{Message: "some message"}
		s.False(r.IsToolLevel())
	})
}

func (s *ConfirmationRuleSuite) TestIsKubeLevel() {
	s.Run("true when verb is set", func() {
		r := ConfirmationRule{Verb: "delete"}
		s.True(r.IsKubeLevel())
	})
	s.Run("true when kind is set", func() {
		r := ConfirmationRule{Kind: "Secret"}
		s.True(r.IsKubeLevel())
	})
	s.Run("true when group is set", func() {
		r := ConfirmationRule{Group: "apps"}
		s.True(r.IsKubeLevel())
	})
	s.Run("true when version is set", func() {
		r := ConfirmationRule{Version: "v1"}
		s.True(r.IsKubeLevel())
	})
	s.Run("true when namespace is set", func() {
		r := ConfirmationRule{Namespace: "kube-system"}
		s.True(r.IsKubeLevel())
	})
	s.Run("true when name is set", func() {
		r := ConfirmationRule{Name: "my-secret"}
		s.True(r.IsKubeLevel())
	})
	s.Run("false when only tool fields are set", func() {
		r := ConfirmationRule{Tool: "helm_uninstall"}
		s.False(r.IsKubeLevel())
	})
	s.Run("false when no fields are set", func() {
		r := ConfirmationRule{Message: "some message"}
		s.False(r.IsKubeLevel())
	})
}

func (s *ConfirmationRuleSuite) TestValidate() {
	s.Run("valid tool-level rule", func() {
		r := ConfirmationRule{Tool: "helm_uninstall", Message: "uninstall"}
		s.NoError(r.Validate())
	})
	s.Run("valid kube-level rule", func() {
		r := ConfirmationRule{Verb: "delete", Kind: "Pod", Message: "delete pod"}
		s.NoError(r.Validate())
	})
	s.Run("valid kube-level rule with namespace", func() {
		r := ConfirmationRule{Verb: "delete", Namespace: "kube-system", Message: "delete"}
		s.NoError(r.Validate())
	})
	s.Run("error when tool and verb are both set", func() {
		r := ConfirmationRule{Tool: "helm_uninstall", Verb: "delete", Message: "mixed"}
		s.Error(r.Validate())
	})
	s.Run("error when destructive and kind are both set", func() {
		r := ConfirmationRule{Destructive: ptr.To(true), Kind: "Secret", Message: "mixed"}
		s.Error(r.Validate())
	})
	s.Run("error when tool and group are both set", func() {
		r := ConfirmationRule{Tool: "some_tool", Group: "apps", Message: "mixed"}
		s.Error(r.Validate())
	})
	s.Run("error when tool and namespace are both set", func() {
		r := ConfirmationRule{Tool: "some_tool", Namespace: "kube-system", Message: "mixed"}
		s.Error(r.Validate())
	})
	s.Run("error when no level fields are set", func() {
		r := ConfirmationRule{Message: "generic"}
		s.Error(r.Validate())
	})
}

func TestConfirmationRule(t *testing.T) {
	suite.Run(t, new(ConfirmationRuleSuite))
}
