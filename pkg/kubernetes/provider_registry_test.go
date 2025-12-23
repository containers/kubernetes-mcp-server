package kubernetes

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
)

type ProviderRegistryTestSuite struct {
	BaseProviderSuite
}

func (s *ProviderRegistryTestSuite) TestRegisterProvider() {
	s.Run("With no pre-existing provider, registers the provider", func() {
		RegisterProvider("test-strategy", func(cfg api.BaseConfig) (Provider, error) {
			return nil, nil
		})
		_, exists := providerFactories["test-strategy"]
		s.True(exists, "Provider should be registered")
	})
	s.Run("With pre-existing provider, panics", func() {
		RegisterProvider("test-pre-existent", func(cfg api.BaseConfig) (Provider, error) {
			return nil, nil
		})
		s.Panics(func() {
			RegisterProvider("test-pre-existent", func(cfg api.BaseConfig) (Provider, error) {
				return nil, nil
			})
		}, "Registering a provider with an existing strategy should panic")
	})
}

func (s *ProviderRegistryTestSuite) TestGetRegisteredStrategies() {
	s.Run("With no registered providers, returns empty list", func() {
		providerFactories = make(map[string]ProviderFactory)
		strategies := GetRegisteredStrategies()
		s.Empty(strategies, "No strategies should be registered")
	})
	s.Run("With multiple registered providers, returns sorted list", func() {
		providerFactories = make(map[string]ProviderFactory)
		RegisterProvider("foo-strategy", func(cfg api.BaseConfig) (Provider, error) {
			return nil, nil
		})
		RegisterProvider("bar-strategy", func(cfg api.BaseConfig) (Provider, error) {
			return nil, nil
		})
		strategies := GetRegisteredStrategies()
		expected := []string{"bar-strategy", "foo-strategy"}
		s.Equal(expected, strategies, "Strategies should be sorted alphabetically")
	})
}

func TestProviderRegistry(t *testing.T) {
	suite.Run(t, new(ProviderRegistryTestSuite))
}
