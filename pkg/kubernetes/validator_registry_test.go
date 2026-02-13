package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	authv1client "k8s.io/client-go/kubernetes/typed/authorization/v1"
)

type ValidatorRegistryTestSuite struct {
	suite.Suite
}

func (s *ValidatorRegistryTestSuite) TestCreateValidatorsReturnsRegisteredValidators() {
	providers := ValidatorProviders{
		RestMapper: func() meta.RESTMapper { return nil },
		Discovery:  func() discovery.DiscoveryInterface { return nil },
		AuthClient: func() authv1client.AuthorizationV1Interface { return nil },
	}

	validators := CreateValidators(providers)

	s.GreaterOrEqual(len(validators), 3, "Expected at least 3 validators (resource, schema, rbac)")

	names := make(map[string]bool)
	for _, v := range validators {
		names[v.Name()] = true
	}

	s.True(names["resource"], "Expected resource validator to be registered")
	s.True(names["schema"], "Expected schema validator to be registered")
	s.True(names["rbac"], "Expected rbac validator to be registered")
}

func (s *ValidatorRegistryTestSuite) TestCreateValidatorsWithNilProviders() {
	providers := ValidatorProviders{
		RestMapper: nil,
		Discovery:  nil,
		AuthClient: nil,
	}

	// Should not panic
	s.NotPanics(func() {
		validators := CreateValidators(providers)
		s.NotEmpty(validators, "Expected validators to be created even with nil providers")
	})
}

func TestValidatorRegistry(t *testing.T) {
	suite.Run(t, new(ValidatorRegistryTestSuite))
}
