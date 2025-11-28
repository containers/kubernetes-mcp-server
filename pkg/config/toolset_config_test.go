package config

import (
	"context"
	"errors"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/suite"
)

type ToolsetConfigSuite struct {
	BaseConfigSuite
	originalToolsetConfigRegistry *extendedConfigRegistry
}

func (s *ToolsetConfigSuite) SetupTest() {
	s.originalToolsetConfigRegistry = toolsetConfigRegistry
	toolsetConfigRegistry = newExtendedConfigRegistry()
}

func (s *ToolsetConfigSuite) TearDownTest() {
	toolsetConfigRegistry = s.originalToolsetConfigRegistry
}

type ToolsetConfigForTest struct {
	Enabled  bool   `toml:"enabled"`
	Endpoint string `toml:"endpoint"`
	Timeout  int    `toml:"timeout"`
}

var _ Extended = (*ToolsetConfigForTest)(nil)

func (t *ToolsetConfigForTest) Validate() error {
	if t.Endpoint == "force-error" {
		return errors.New("validation error forced by test")
	}
	return nil
}

func toolsetConfigForTestParser(_ context.Context, primitive toml.Primitive, md toml.MetaData) (Extended, error) {
	var toolsetConfigForTest ToolsetConfigForTest
	if err := md.PrimitiveDecode(primitive, &toolsetConfigForTest); err != nil {
		return nil, err
	}
	return &toolsetConfigForTest, nil
}

func (s *ToolsetConfigSuite) TestRegisterToolsetConfig() {
	s.Run("panics when registering duplicate toolset config parser", func() {
		s.Panics(func() {
			RegisterToolsetConfig("test-toolset", toolsetConfigForTestParser)
			RegisterToolsetConfig("test-toolset", toolsetConfigForTestParser)
		}, "Expected panic when registering duplicate toolset config parser")
	})
}

func (s *ToolsetConfigSuite) TestReadConfigValid() {
	RegisterToolsetConfig("test-toolset", toolsetConfigForTestParser)
	validConfigPath := s.writeConfig(`
		[toolset_configs.test-toolset]
		enabled = true
		endpoint = "https://example.com"
		timeout = 30
	`)

	config, err := Read(validConfigPath, "")
	s.Run("returns no error for valid file with registered toolset config", func() {
		s.Require().NoError(err, "Expected no error for valid file, got %v", err)
	})
	s.Run("returns config for valid file with registered toolset config", func() {
		s.Require().NotNil(config, "Expected non-nil config for valid file")
	})
	s.Run("parses toolset config correctly", func() {
		toolsetConfig, ok := config.GetToolsetConfig("test-toolset")
		s.Require().True(ok, "Expected to find toolset config for 'test-toolset'")
		s.Require().NotNil(toolsetConfig, "Expected non-nil toolset config for 'test-toolset'")
		testToolsetConfig, ok := toolsetConfig.(*ToolsetConfigForTest)
		s.Require().True(ok, "Expected toolset config to be of type *ToolsetConfigForTest")
		s.Equal(true, testToolsetConfig.Enabled, "Expected Enabled to be true")
		s.Equal("https://example.com", testToolsetConfig.Endpoint, "Expected Endpoint to be 'https://example.com'")
		s.Equal(30, testToolsetConfig.Timeout, "Expected Timeout to be 30")
	})
}

func (s *ToolsetConfigSuite) TestReadConfigInvalidToolsetConfig() {
	RegisterToolsetConfig("test-toolset", toolsetConfigForTestParser)
	invalidConfigPath := s.writeConfig(`
		[toolset_configs.test-toolset]
		enabled = true
		endpoint = "force-error"
		timeout = 30
	`)

	config, err := Read(invalidConfigPath, "")
	s.Run("returns error for invalid toolset config", func() {
		s.Require().NotNil(err, "Expected error for invalid toolset config, got nil")
		s.ErrorContains(err, "validation error forced by test", "Expected validation error from toolset config")
	})
	s.Run("returns nil config for invalid toolset config", func() {
		s.Nil(config, "Expected nil config for invalid toolset config")
	})
}

func (s *ToolsetConfigSuite) TestReadConfigUnregisteredToolsetConfig() {
	unregisteredConfigPath := s.writeConfig(`
		[toolset_configs.unregistered-toolset]
		enabled = true
		endpoint = "https://example.com"
		timeout = 30
	`)

	config, err := Read(unregisteredConfigPath, "")
	s.Run("returns no error for unregistered toolset config", func() {
		s.Require().NoError(err, "Expected no error for unregistered toolset config, got %v", err)
	})
	s.Run("returns config for unregistered toolset config", func() {
		s.Require().NotNil(config, "Expected non-nil config for unregistered toolset config")
	})
	s.Run("does not parse unregistered toolset config", func() {
		_, ok := config.GetToolsetConfig("unregistered-toolset")
		s.Require().False(ok, "Expected no toolset config for unregistered toolset")
	})
}

func TestToolsetConfig(t *testing.T) {
	suite.Run(t, new(ToolsetConfigSuite))
}
