package kubernetes

import (
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"k8s.io/client-go/discovery"
	authv1client "k8s.io/client-go/kubernetes/typed/authorization/v1"
)

// ValidatorProviders holds the providers needed to create validators.
type ValidatorProviders struct {
	Discovery  func() discovery.DiscoveryInterface
	AuthClient func() authv1client.AuthorizationV1Interface
}

// ValidatorFactory creates a validator given the providers.
type ValidatorFactory func(ValidatorProviders) api.HTTPValidator

var validatorFactories []ValidatorFactory

// RegisterValidator adds a validator factory to the registry.
func RegisterValidator(factory ValidatorFactory) {
	validatorFactories = append(validatorFactories, factory)
}

// CreateValidators creates all registered validators with the given providers.
func CreateValidators(providers ValidatorProviders) []api.HTTPValidator {
	validators := make([]api.HTTPValidator, 0, len(validatorFactories))
	for _, factory := range validatorFactories {
		validators = append(validators, factory(providers))
	}
	return validators
}

func init() {
	RegisterValidator(func(p ValidatorProviders) api.HTTPValidator {
		return NewSchemaValidator(p.Discovery)
	})
	RegisterValidator(func(p ValidatorProviders) api.HTTPValidator {
		return NewRBACValidator(p.AuthClient)
	})
}
