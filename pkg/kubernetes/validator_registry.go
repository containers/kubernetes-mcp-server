package kubernetes

import (
	"fmt"
	"slices"

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

var validatorReg = &validatorRegistry{factories: make(map[string]ValidatorFactory)}

// RegisterValidator adds a validator factory to the registry.
// Panics if a validator is already registered with the given name.
func RegisterValidator(name string, factory ValidatorFactory) {
	validatorReg.register(name, factory)
}

// CreateValidators creates all registered validators with the given providers.
func CreateValidators(providers ValidatorProviders) []api.HTTPValidator {
	return validatorReg.createAll(providers)
}

type validatorRegistry struct {
	factories map[string]ValidatorFactory
}

func (r *validatorRegistry) register(name string, factory ValidatorFactory) {
	if _, exists := r.factories[name]; exists {
		panic(fmt.Sprintf("validator already registered for name '%s'", name))
	}
	r.factories[name] = factory
}

func (r *validatorRegistry) createAll(providers ValidatorProviders) []api.HTTPValidator {
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	slices.Sort(names)
	validators := make([]api.HTTPValidator, 0, len(r.factories))
	for _, name := range names {
		validators = append(validators, r.factories[name](providers))
	}
	return validators
}

func init() {
	RegisterValidator("schema", func(p ValidatorProviders) api.HTTPValidator {
		return NewSchemaValidator(p.Discovery)
	})
	RegisterValidator("rbac", func(p ValidatorProviders) api.HTTPValidator {
		return NewRBACValidator(p.AuthClient)
	})
}
