package config

import (
	"errors"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/suite"
)

type ProviderConfigSuite struct {
	BaseConfigSuite
	originalProviderConfigParsers map[string]ProviderConfigParser
}

func (s *ProviderConfigSuite) SetupTest() {
	s.originalProviderConfigParsers = make(map[string]ProviderConfigParser)
	for k, v := range providerConfigParsers {
		s.originalProviderConfigParsers[k] = v
	}
}

func (s *ProviderConfigSuite) TearDownTest() {
	providerConfigParsers = make(map[string]ProviderConfigParser)
	for k, v := range s.originalProviderConfigParsers {
		providerConfigParsers[k] = v
	}
}

type ProviderConfigForTest struct {
	BoolProp bool   `toml:"bool_prop"`
	StrProp  string `toml:"str_prop"`
	IntProp  int    `toml:"int_prop"`
}

var _ ProviderConfig = (*ProviderConfigForTest)(nil)

func (p *ProviderConfigForTest) Validate() error {
	if p.StrProp == "force-error" {
		return errors.New("validation error forced by test")
	}
	return nil
}

func providerConfigForTestParser(primitive toml.Primitive, md toml.MetaData) (ProviderConfig, error) {
	var providerConfigForTest ProviderConfigForTest
	if err := md.PrimitiveDecode(primitive, &providerConfigForTest); err != nil {
		return nil, err
	}
	return &providerConfigForTest, nil
}

func (s *ProviderConfigSuite) TestRegisterProviderConfig() {
	s.Run("panics when registering duplicate provider config parser", func() {
		s.Panics(func() {
			RegisterProviderConfig("test", providerConfigForTestParser)
			RegisterProviderConfig("test", providerConfigForTestParser)
		}, "Expected panic when registering duplicate provider config parser")
	})
}

func (s *ProviderConfigSuite) TestReadConfigValid() {
	RegisterProviderConfig("test", providerConfigForTestParser)
	validConfigPath := s.writeConfig(`
		cluster_provider_strategy = "test"
		[cluster_provider_configs.test]
		bool_prop = true
		str_prop = "a string"
		int_prop = 42
	`)

	config, err := Read(validConfigPath)
	s.Run("returns no error for valid file with registered provider config", func() {
		s.Require().NoError(err, "Expected no error for valid file, got %v", err)
	})
	s.Run("returns config for valid file with registered provider config", func() {
		s.Require().NotNil(config, "Expected non-nil config for valid file")
	})
	s.Run("parses provider config correctly", func() {
		providerConfig, ok := config.GetProviderConfig("test")
		s.Require().True(ok, "Expected to find provider config for strategy 'test'")
		s.Require().NotNil(providerConfig, "Expected non-nil provider config for strategy 'test'")
		testProviderConfig, ok := providerConfig.(*ProviderConfigForTest)
		s.Require().True(ok, "Expected provider config to be of type *ProviderConfigForTest")
		s.Equal(true, testProviderConfig.BoolProp, "Expected BoolProp to be true")
		s.Equal("a string", testProviderConfig.StrProp, "Expected StrProp to be 'a string'")
		s.Equal(42, testProviderConfig.IntProp, "Expected IntProp to be 42")
	})
}

func (s *ProviderConfigSuite) TestReadConfigInvalidProviderConfig() {
	RegisterProviderConfig("test", providerConfigForTestParser)
	invalidConfigPath := s.writeConfig(`
		cluster_provider_strategy = "test"
		[cluster_provider_configs.test]
		bool_prop = true
		str_prop = "force-error"
		int_prop = 42
	`)

	config, err := Read(invalidConfigPath)
	s.Run("returns error for invalid provider config", func() {
		s.Require().NotNil(err, "Expected error for invalid provider config, got nil")
		s.ErrorContains(err, "validation error forced by test", "Expected validation error from provider config")
	})
	s.Run("returns nil config for invalid provider config", func() {
		s.Nil(config, "Expected nil config for invalid provider config")
	})
}

func (s *ProviderConfigSuite) TestReadConfigUnregisteredProviderConfig() {
	invalidConfigPath := s.writeConfig(`
		cluster_provider_strategy = "unregistered"
		[cluster_provider_configs.unregistered]
		bool_prop = true
		str_prop = "a string"
		int_prop = 42
	`)

	config, err := Read(invalidConfigPath)
	s.Run("returns no error for unregistered provider config", func() {
		s.Require().NoError(err, "Expected no error for unregistered provider config, got %v", err)
	})
	s.Run("returns config for unregistered provider config", func() {
		s.Require().NotNil(config, "Expected non-nil config for unregistered provider config")
	})
	s.Run("does not parse unregistered provider config", func() {
		_, ok := config.GetProviderConfig("unregistered")
		s.Require().False(ok, "Expected no provider config for unregistered strategy")
	})
}

func (s *ProviderConfigSuite) TestReadConfigParserError() {
	RegisterProviderConfig("test", func(primitive toml.Primitive, md toml.MetaData) (ProviderConfig, error) {
		return nil, errors.New("parser error forced by test")
	})
	invalidConfigPath := s.writeConfig(`
		cluster_provider_strategy = "test"
		[cluster_provider_configs.test]
		bool_prop = true
		str_prop = "a string"
		int_prop = 42
	`)

	config, err := Read(invalidConfigPath)
	s.Run("returns error for provider config parser error", func() {
		s.Require().NotNil(err, "Expected error for provider config parser error, got nil")
		s.ErrorContains(err, "parser error forced by test", "Expected parser error from provider config")
	})
	s.Run("returns nil config for provider config parser error", func() {
		s.Nil(config, "Expected nil config for provider config parser error")
	})
}

func TestProviderConfig(t *testing.T) {
	suite.Run(t, new(ProviderConfigSuite))
}
