package tokenexchange

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type TokenExchangerRegistryTestSuite struct {
	suite.Suite
}

func (s *TokenExchangerRegistryTestSuite) TestGetTokenExchanger() {
	s.Run("returns keycloak-v1 exchanger", func() {
		exchanger, ok := GetTokenExchanger(StrategyKeycloakV1)
		s.True(ok, "Expected keycloak-v1 exchanger to be registered")
		s.NotNil(exchanger, "Expected keycloak-v1 exchanger to be non-nil")
	})
	s.Run("returns rfc8693 exchanger", func() {
		exchanger, ok := GetTokenExchanger(StrategyRFC8693)
		s.True(ok, "Expected rfc8693 exchanger to be registered")
		s.NotNil(exchanger, "Expected rfc8693 exchanger to be non-nil")
	})
	s.Run("returns entra-obo exchanger", func() {
		exchanger, ok := GetTokenExchanger(StrategyEntraOBO)
		s.True(ok, "Expected entra-obo exchanger to be registered")
		s.NotNil(exchanger, "Expected entra-obo exchanger to be non-nil")
	})
	s.Run("returns false for unregistered strategy", func() {
		exchanger, ok := GetTokenExchanger("non-existent")
		s.False(ok, "Expected false for non-existent strategy")
		s.Nil(exchanger, "Expected nil for non-existent strategy")
	})
}

func (s *TokenExchangerRegistryTestSuite) TestRegisterTokenExchanger() {
	s.Run("panics on duplicate registration", func() {
		s.Panics(func() {
			RegisterTokenExchanger(StrategyKeycloakV1, &keycloakV1Exchanger{})
		}, "Expected panic on duplicate registration")
	})
}

func TestTokenExchangerRegistry(t *testing.T) {
	suite.Run(t, new(TokenExchangerRegistryTestSuite))
}
