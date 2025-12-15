package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
)

type ProviderConfigSuite struct {
	BaseConfigSuite
	originalProviderConfigRegistry *extendedConfigRegistry
}

func (s *ProviderConfigSuite) SetupTest() {
	s.originalProviderConfigRegistry = providerConfigRegistry
	providerConfigRegistry = newExtendedConfigRegistry()
}

func (s *ProviderConfigSuite) TearDownTest() {
	providerConfigRegistry = s.originalProviderConfigRegistry
}

type ProviderConfigForTest struct {
	BoolProp bool   `toml:"bool_prop"`
	StrProp  string `toml:"str_prop"`
	IntProp  int    `toml:"int_prop"`
}

var _ api.ExtendedConfig = (*ProviderConfigForTest)(nil)

func (p *ProviderConfigForTest) Validate() error {
	if p.StrProp == "force-error" {
		return errors.New("validation error forced by test")
	}
	return nil
}

func providerConfigForTestParser(_ context.Context, primitive toml.Primitive, md toml.MetaData) (api.ExtendedConfig, error) {
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

	config, err := Read(validConfigPath, "")
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

	config, err := Read(invalidConfigPath, "")
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

	config, err := Read(invalidConfigPath, "")
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
	RegisterProviderConfig("test", func(ctx context.Context, primitive toml.Primitive, md toml.MetaData) (api.ExtendedConfig, error) {
		return nil, errors.New("parser error forced by test")
	})
	invalidConfigPath := s.writeConfig(`
		cluster_provider_strategy = "test"
		[cluster_provider_configs.test]
		bool_prop = true
		str_prop = "a string"
		int_prop = 42
	`)

	config, err := Read(invalidConfigPath, "")
	s.Run("returns error for provider config parser error", func() {
		s.Require().NotNil(err, "Expected error for provider config parser error, got nil")
		s.ErrorContains(err, "parser error forced by test", "Expected parser error from provider config")
	})
	s.Run("returns nil config for provider config parser error", func() {
		s.Nil(config, "Expected nil config for provider config parser error")
	})
}

func (s *ProviderConfigSuite) TestConfigDirPathInContext() {
	var capturedDirPath string
	RegisterProviderConfig("test", func(ctx context.Context, primitive toml.Primitive, md toml.MetaData) (api.ExtendedConfig, error) {
		capturedDirPath = ConfigDirPathFromContext(ctx)
		var providerConfigForTest ProviderConfigForTest
		if err := md.PrimitiveDecode(primitive, &providerConfigForTest); err != nil {
			return nil, err
		}
		return &providerConfigForTest, nil
	})
	configPath := s.writeConfig(`
		cluster_provider_strategy = "test"
		[cluster_provider_configs.test]
		bool_prop = true
		str_prop = "a string"
		int_prop = 42
	`)

	absConfigPath, err := filepath.Abs(configPath)
	s.Require().NoError(err, "test error: getting the absConfigPath should not fail")

	_, err = Read(configPath, "")
	s.Run("provides config directory path in context to parser", func() {
		s.Require().NoError(err, "Expected no error reading config")
		s.NotEmpty(capturedDirPath, "Expected non-empty directory path in context")
		s.Equal(filepath.Dir(absConfigPath), capturedDirPath, "Expected directory path to match config file directory")
	})
}

func (s *ProviderConfigSuite) TestExtendedConfigMergingAcrossDropIns() {
	// Test that extended configs (cluster_provider_configs) are properly merged
	// when scattered across multiple drop-in files
	RegisterProviderConfig("test", providerConfigForTestParser)

	tempDir := s.T().TempDir()

	// Create main config with initial provider config
	mainConfigPath := filepath.Join(tempDir, "config.toml")
	err := os.WriteFile(mainConfigPath, []byte(`
		cluster_provider_strategy = "test"
		[cluster_provider_configs.test]
		bool_prop = false
		str_prop = "from-main"
		int_prop = 1
	`), 0644)
	s.Require().NoError(err)

	// Create drop-in directory
	dropInDir := filepath.Join(tempDir, "conf.d")
	s.Require().NoError(os.Mkdir(dropInDir, 0755))

	// First drop-in overrides some fields
	err = os.WriteFile(filepath.Join(dropInDir, "10-override.toml"), []byte(`
		[cluster_provider_configs.test]
		bool_prop = true
		int_prop = 10
	`), 0644)
	s.Require().NoError(err)

	// Second drop-in overrides other fields
	err = os.WriteFile(filepath.Join(dropInDir, "20-final.toml"), []byte(`
		[cluster_provider_configs.test]
		str_prop = "from-drop-in"
		int_prop = 42
	`), 0644)
	s.Require().NoError(err)

	config, err := Read(mainConfigPath, "")
	s.Require().NoError(err)
	s.Require().NotNil(config)

	providerConfig, ok := config.GetProviderConfig("test")
	s.Require().True(ok, "Expected to find provider config")

	testConfig, ok := providerConfig.(*ProviderConfigForTest)
	s.Require().True(ok, "Expected provider config to be *ProviderConfigForTest")

	s.Run("merges bool_prop from first drop-in", func() {
		s.True(testConfig.BoolProp, "bool_prop should be true from 10-override.toml")
	})

	s.Run("merges str_prop from second drop-in", func() {
		s.Equal("from-drop-in", testConfig.StrProp, "str_prop should be from 20-final.toml")
	})

	s.Run("last drop-in wins for int_prop", func() {
		s.Equal(42, testConfig.IntProp, "int_prop should be 42 from 20-final.toml")
	})
}

func (s *ProviderConfigSuite) TestExtendedConfigFromDropInOnly() {
	// Test that extended configs work when defined only in drop-in files (not in main config)
	RegisterProviderConfig("test", providerConfigForTestParser)

	tempDir := s.T().TempDir()

	// Create main config WITHOUT provider config
	mainConfigPath := filepath.Join(tempDir, "config.toml")
	err := os.WriteFile(mainConfigPath, []byte(`
		cluster_provider_strategy = "test"
		log_level = 1
	`), 0644)
	s.Require().NoError(err)

	// Create drop-in directory
	dropInDir := filepath.Join(tempDir, "conf.d")
	s.Require().NoError(os.Mkdir(dropInDir, 0755))

	// Drop-in defines the provider config
	err = os.WriteFile(filepath.Join(dropInDir, "10-provider.toml"), []byte(`
		[cluster_provider_configs.test]
		bool_prop = true
		str_prop = "from-drop-in-only"
		int_prop = 99
	`), 0644)
	s.Require().NoError(err)

	config, err := Read(mainConfigPath, "")
	s.Require().NoError(err)
	s.Require().NotNil(config)

	providerConfig, ok := config.GetProviderConfig("test")
	s.Require().True(ok, "Expected to find provider config from drop-in")

	testConfig, ok := providerConfig.(*ProviderConfigForTest)
	s.Require().True(ok)

	s.Run("loads extended config from drop-in only", func() {
		s.True(testConfig.BoolProp)
		s.Equal("from-drop-in-only", testConfig.StrProp)
		s.Equal(99, testConfig.IntProp)
	})
}

func (s *ProviderConfigSuite) TestStandaloneConfigDirWithExtendedConfig() {
	// Test that extended configs work with standalone --config-dir (no main config)
	RegisterProviderConfig("test", providerConfigForTestParser)

	tempDir := s.T().TempDir()

	// Create drop-in files only (no main config)
	err := os.WriteFile(filepath.Join(tempDir, "10-base.toml"), []byte(`
		cluster_provider_strategy = "test"
		[cluster_provider_configs.test]
		bool_prop = false
		str_prop = "base"
		int_prop = 1
	`), 0644)
	s.Require().NoError(err)

	err = os.WriteFile(filepath.Join(tempDir, "20-override.toml"), []byte(`
		[cluster_provider_configs.test]
		bool_prop = true
		int_prop = 100
	`), 0644)
	s.Require().NoError(err)

	// Read with standalone config-dir (empty config path)
	config, err := Read("", tempDir)
	s.Require().NoError(err)
	s.Require().NotNil(config)

	providerConfig, ok := config.GetProviderConfig("test")
	s.Require().True(ok, "Expected to find provider config in standalone mode")

	testConfig, ok := providerConfig.(*ProviderConfigForTest)
	s.Require().True(ok)

	s.Run("merges extended config in standalone mode", func() {
		s.True(testConfig.BoolProp, "bool_prop should be true from 20-override.toml")
		s.Equal("base", testConfig.StrProp, "str_prop should be 'base' from 10-base.toml")
		s.Equal(100, testConfig.IntProp, "int_prop should be 100 from 20-override.toml")
	})
}

func (s *ProviderConfigSuite) TestConfigDirPathInContextStandalone() {
	// Test that configDirPath is correctly set in context for standalone --config-dir
	var capturedDirPath string
	RegisterProviderConfig("test", func(ctx context.Context, primitive toml.Primitive, md toml.MetaData) (api.ExtendedConfig, error) {
		capturedDirPath = ConfigDirPathFromContext(ctx)
		var providerConfigForTest ProviderConfigForTest
		if err := md.PrimitiveDecode(primitive, &providerConfigForTest); err != nil {
			return nil, err
		}
		return &providerConfigForTest, nil
	})

	tempDir := s.T().TempDir()

	err := os.WriteFile(filepath.Join(tempDir, "10-config.toml"), []byte(`
		cluster_provider_strategy = "test"
		[cluster_provider_configs.test]
		bool_prop = true
		str_prop = "test"
		int_prop = 1
	`), 0644)
	s.Require().NoError(err)

	absTempDir, err := filepath.Abs(tempDir)
	s.Require().NoError(err)

	_, err = Read("", tempDir)
	s.Run("provides config directory path in context for standalone mode", func() {
		s.Require().NoError(err)
		s.NotEmpty(capturedDirPath, "Expected non-empty directory path in context")
		s.Equal(absTempDir, capturedDirPath, "Expected directory path to match config-dir")
	})
}

func TestProviderConfig(t *testing.T) {
	suite.Run(t, new(ProviderConfigSuite))
}
