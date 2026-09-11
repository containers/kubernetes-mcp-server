package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type KubeConfigProviderConfigTestSuite struct {
	suite.Suite
}

func (s *KubeConfigProviderConfigTestSuite) TestValidate() {
	s.Run("valid config with strategy and targets", func() {
		cfg := &KubeConfigProviderConfig{
			Strategy: "keycloak-v2",
			Targets: map[string]KubeConfigTargetConfig{
				"spoke": {Audience: "spoke-cluster"},
			},
		}
		s.NoError(cfg.Validate())
	})

	s.Run("valid config with empty strategy and no targets", func() {
		cfg := &KubeConfigProviderConfig{}
		s.NoError(cfg.Validate())
	})

	s.Run("rejects targets without strategy", func() {
		cfg := &KubeConfigProviderConfig{
			Targets: map[string]KubeConfigTargetConfig{
				"spoke": {Audience: "spoke-cluster"},
			},
		}
		err := cfg.Validate()
		s.Error(err)
		s.Contains(err.Error(), "strategy is required")
	})

	s.Run("valid config with no targets", func() {
		cfg := &KubeConfigProviderConfig{
			Strategy: "keycloak-v2",
		}
		s.NoError(cfg.Validate())
	})

	s.Run("rejects nil config", func() {
		var cfg *KubeConfigProviderConfig
		s.Error(cfg.Validate())
	})

	s.Run("rejects unknown strategy", func() {
		cfg := &KubeConfigProviderConfig{
			Strategy: "nonexistent",
		}
		err := cfg.Validate()
		s.Error(err)
		s.Contains(err.Error(), "nonexistent")
	})

	s.Run("rejects target without audience", func() {
		cfg := &KubeConfigProviderConfig{
			Strategy: "keycloak-v2",
			Targets: map[string]KubeConfigTargetConfig{
				"spoke": {Scopes: []string{"mcp:spoke"}},
			},
		}
		err := cfg.Validate()
		s.Error(err)
		s.Contains(err.Error(), "spoke")
		s.Contains(err.Error(), "audience")
	})

	s.Run("accepts target with all optional fields", func() {
		cfg := &KubeConfigProviderConfig{
			Strategy: "rfc8693",
			Targets: map[string]KubeConfigTargetConfig{
				"spoke": {
					Audience:     "spoke-cluster",
					Scopes:       []string{"mcp:spoke"},
					ClientId:     "spoke-client",
					ClientSecret: "spoke-secret",
				},
			},
		}
		s.NoError(cfg.Validate())
	})

	s.Run("validates multiple targets independently", func() {
		cfg := &KubeConfigProviderConfig{
			Strategy: "keycloak-v2",
			Targets: map[string]KubeConfigTargetConfig{
				"spoke-1": {Audience: "spoke-1-audience"},
				"spoke-2": {},
			},
		}
		err := cfg.Validate()
		s.Error(err)
		s.Contains(err.Error(), "spoke-2")
	})
}

func (s *KubeConfigProviderConfigTestSuite) TestTargetConfigDefaults() {
	s.Run("unset fields are zero-valued for fallback", func() {
		cfg := KubeConfigTargetConfig{
			Audience: "spoke-cluster",
		}
		s.Empty(cfg.Scopes)
		s.Empty(cfg.ClientId)
		s.Empty(cfg.ClientSecret)
	})
}

func TestKubeConfigProviderConfig(t *testing.T) {
	suite.Run(t, new(KubeConfigProviderConfigTestSuite))
}
