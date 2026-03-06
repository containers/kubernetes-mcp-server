package kubernetes

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/discovery"
	authv1client "k8s.io/client-go/kubernetes/typed/authorization/v1"
)

type ValidatorRegistryTestSuite struct {
	suite.Suite
}

func (s *ValidatorRegistryTestSuite) TestCreateValidatorsReturnsRegisteredValidators() {
	providers := ValidatorProviders{
		Discovery:  func() discovery.DiscoveryInterface { return nil },
		AuthClient: func() authv1client.AuthorizationV1Interface { return nil },
	}

	validators := CreateValidators(providers)

	s.GreaterOrEqual(len(validators), 2, "Expected at least 2 validators (schema, rbac)")

	names := make(map[string]bool)
	for _, v := range validators {
		names[v.Name()] = true
	}

	s.True(names["schema"], "Expected schema validator to be registered")
	s.True(names["rbac"], "Expected rbac validator to be registered")
}

func (s *ValidatorRegistryTestSuite) TestCreateValidatorsWithNilProviders() {
	providers := ValidatorProviders{
		Discovery:  nil,
		AuthClient: nil,
	}

	// Should not panic
	s.NotPanics(func() {
		validators := CreateValidators(providers)
		s.NotEmpty(validators, "Expected validators to be created even with nil providers")
	})
}

func (s *ValidatorRegistryTestSuite) TestRegisterValidatorPanicsOnDuplicate() {
	s.T().Skip("Will be enabled after refactoring validator registry to wrapped struct pattern")
	s.Panics(func() {
		RegisterValidator(func(p ValidatorProviders) api.HTTPValidator {
			return NewSchemaValidator(p.Discovery)
		})
	}, "Expected panic on duplicate validator registration")
}

func TestValidatorRegistry(t *testing.T) {
	suite.Run(t, new(ValidatorRegistryTestSuite))
}
