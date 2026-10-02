package kubernetes

import (
	"fmt"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/suite"
)

type KubeConfigProviderConfigTestSuite struct {
	suite.Suite
}

func (s *KubeConfigProviderConfigTestSuite) TestValidate() {
	s.Run("valid config with strategy and targets", func() {
		cfg := &KubeConfigProviderConfig{
			Strategy: "rfc8693",
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
			Strategy: "rfc8693",
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
			Strategy: "rfc8693",
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
			Strategy: "rfc8693",
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

// TestDottedContextNameRequiresTomlQuoting documents that a kubeconfig context
// name containing dots must be TOML-quoted in the table header. An unquoted
// dotted key creates nested TOML tables rather than a flat Targets map entry,
// silently dropping the per-target token exchange configuration.
func (s *KubeConfigProviderConfigTestSuite) TestDottedContextNameRequiresTomlQuoting() {
	ctxName := "context.with.dots"

	// Unquoted: [targets.context.with.dots] → TOML nested tables, NOT a flat entry.
	unquotedTOML := fmt.Sprintf(`
strategy = "rfc8693"
[targets.%s]
audience = "spoke"
`, ctxName)

	var unquotedCfg KubeConfigProviderConfig
	_, err := toml.Decode(unquotedTOML, &unquotedCfg)
	s.Require().NoError(err, "TOML parses without error even though the key is wrong")

	_, found := unquotedCfg.Targets[ctxName]
	s.False(found,
		"unquoted dotted key creates nested TOML tables, not a flat Targets entry")

	// Quoted: [targets."context.with.dots"] → correct flat entry.
	quotedTOML := fmt.Sprintf(`
strategy = "rfc8693"
[targets."%s"]
audience = "spoke"
`, ctxName)

	var quotedCfg KubeConfigProviderConfig
	_, err = toml.Decode(quotedTOML, &quotedCfg)
	s.Require().NoError(err)

	_, found = quotedCfg.Targets[ctxName]
	s.True(found,
		`quoted key [targets."`+ctxName+`"] must produce a flat Targets entry`)
}

func TestKubeConfigProviderConfig(t *testing.T) {
	suite.Run(t, new(KubeConfigProviderConfigTestSuite))
}
