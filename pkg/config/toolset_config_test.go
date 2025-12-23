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

var _ api.ExtendedConfig = (*ToolsetConfigForTest)(nil)

func (t *ToolsetConfigForTest) Validate() error {
	if t.Endpoint == "force-error" {
		return errors.New("validation error forced by test")
	}
	return nil
}

func toolsetConfigForTestParser(_ context.Context, primitive toml.Primitive, md toml.MetaData) (api.ExtendedConfig, error) {
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

func (s *ToolsetConfigSuite) TestConfigDirPathInContext() {
	var capturedDirPath string
	RegisterToolsetConfig("test-toolset", func(ctx context.Context, primitive toml.Primitive, md toml.MetaData) (api.ExtendedConfig, error) {
		capturedDirPath = ConfigDirPathFromContext(ctx)
		var toolsetConfigForTest ToolsetConfigForTest
		if err := md.PrimitiveDecode(primitive, &toolsetConfigForTest); err != nil {
			return nil, err
		}
		return &toolsetConfigForTest, nil
	})
	configPath := s.writeConfig(`
		[toolset_configs.test-toolset]
		enabled = true
		endpoint = "https://example.com"
		timeout = 30
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

func (s *ToolsetConfigSuite) TestExtendedConfigMergingAcrossDropIns() {
	// Test that extended configs (toolset_configs) are properly merged
	// when scattered across multiple drop-in files
	RegisterToolsetConfig("test-toolset", toolsetConfigForTestParser)

	tempDir := s.T().TempDir()

	// Create main config with initial toolset config
	mainConfigPath := filepath.Join(tempDir, "config.toml")
	err := os.WriteFile(mainConfigPath, []byte(`
		[toolset_configs.test-toolset]
		enabled = false
		endpoint = "from-main"
		timeout = 1
	`), 0644)
	s.Require().NoError(err)

	// Create drop-in directory
	dropInDir := filepath.Join(tempDir, "conf.d")
	s.Require().NoError(os.Mkdir(dropInDir, 0755))

	// First drop-in overrides some fields
	err = os.WriteFile(filepath.Join(dropInDir, "10-override.toml"), []byte(`
		[toolset_configs.test-toolset]
		enabled = true
		timeout = 10
	`), 0644)
	s.Require().NoError(err)

	// Second drop-in overrides other fields
	err = os.WriteFile(filepath.Join(dropInDir, "20-final.toml"), []byte(`
		[toolset_configs.test-toolset]
		endpoint = "from-drop-in"
		timeout = 42
	`), 0644)
	s.Require().NoError(err)

	config, err := Read(mainConfigPath, "")
	s.Require().NoError(err)
	s.Require().NotNil(config)

	toolsetConfig, ok := config.GetToolsetConfig("test-toolset")
	s.Require().True(ok, "Expected to find toolset config")

	testConfig, ok := toolsetConfig.(*ToolsetConfigForTest)
	s.Require().True(ok, "Expected toolset config to be *ToolsetConfigForTest")

	s.Run("merges enabled from first drop-in", func() {
		s.True(testConfig.Enabled, "enabled should be true from 10-override.toml")
	})

	s.Run("merges endpoint from second drop-in", func() {
		s.Equal("from-drop-in", testConfig.Endpoint, "endpoint should be from 20-final.toml")
	})

	s.Run("last drop-in wins for timeout", func() {
		s.Equal(42, testConfig.Timeout, "timeout should be 42 from 20-final.toml")
	})
}

func (s *ToolsetConfigSuite) TestExtendedConfigFromDropInOnly() {
	// Test that extended configs work when defined only in drop-in files (not in main config)
	RegisterToolsetConfig("test-toolset", toolsetConfigForTestParser)

	tempDir := s.T().TempDir()

	// Create main config WITHOUT toolset config
	mainConfigPath := filepath.Join(tempDir, "config.toml")
	err := os.WriteFile(mainConfigPath, []byte(`
		log_level = 1
	`), 0644)
	s.Require().NoError(err)

	// Create drop-in directory
	dropInDir := filepath.Join(tempDir, "conf.d")
	s.Require().NoError(os.Mkdir(dropInDir, 0755))

	// Drop-in defines the toolset config
	err = os.WriteFile(filepath.Join(dropInDir, "10-toolset.toml"), []byte(`
		[toolset_configs.test-toolset]
		enabled = true
		endpoint = "from-drop-in-only"
		timeout = 99
	`), 0644)
	s.Require().NoError(err)

	config, err := Read(mainConfigPath, "")
	s.Require().NoError(err)
	s.Require().NotNil(config)

	toolsetConfig, ok := config.GetToolsetConfig("test-toolset")
	s.Require().True(ok, "Expected to find toolset config from drop-in")

	testConfig, ok := toolsetConfig.(*ToolsetConfigForTest)
	s.Require().True(ok)

	s.Run("loads extended config from drop-in only", func() {
		s.True(testConfig.Enabled)
		s.Equal("from-drop-in-only", testConfig.Endpoint)
		s.Equal(99, testConfig.Timeout)
	})
}

func (s *ToolsetConfigSuite) TestStandaloneConfigDirWithExtendedConfig() {
	// Test that extended configs work with standalone --config-dir (no main config)
	RegisterToolsetConfig("test-toolset", toolsetConfigForTestParser)

	tempDir := s.T().TempDir()

	// Create drop-in files only (no main config)
	err := os.WriteFile(filepath.Join(tempDir, "10-base.toml"), []byte(`
		[toolset_configs.test-toolset]
		enabled = false
		endpoint = "base"
		timeout = 1
	`), 0644)
	s.Require().NoError(err)

	err = os.WriteFile(filepath.Join(tempDir, "20-override.toml"), []byte(`
		[toolset_configs.test-toolset]
		enabled = true
		timeout = 100
	`), 0644)
	s.Require().NoError(err)

	// Read with standalone config-dir (empty config path)
	config, err := Read("", tempDir)
	s.Require().NoError(err)
	s.Require().NotNil(config)

	toolsetConfig, ok := config.GetToolsetConfig("test-toolset")
	s.Require().True(ok, "Expected to find toolset config in standalone mode")

	testConfig, ok := toolsetConfig.(*ToolsetConfigForTest)
	s.Require().True(ok)

	s.Run("merges extended config in standalone mode", func() {
		s.True(testConfig.Enabled, "enabled should be true from 20-override.toml")
		s.Equal("base", testConfig.Endpoint, "endpoint should be 'base' from 10-base.toml")
		s.Equal(100, testConfig.Timeout, "timeout should be 100 from 20-override.toml")
	})
}

func (s *ToolsetConfigSuite) TestConfigDirPathInContextStandalone() {
	// Test that configDirPath is correctly set in context for standalone --config-dir
	var capturedDirPath string
	RegisterToolsetConfig("test-toolset", func(ctx context.Context, primitive toml.Primitive, md toml.MetaData) (api.ExtendedConfig, error) {
		capturedDirPath = ConfigDirPathFromContext(ctx)
		var toolsetConfigForTest ToolsetConfigForTest
		if err := md.PrimitiveDecode(primitive, &toolsetConfigForTest); err != nil {
			return nil, err
		}
		return &toolsetConfigForTest, nil
	})

	tempDir := s.T().TempDir()

	err := os.WriteFile(filepath.Join(tempDir, "10-config.toml"), []byte(`
		[toolset_configs.test-toolset]
		enabled = true
		endpoint = "test"
		timeout = 1
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

func TestToolsetConfig(t *testing.T) {
	suite.Run(t, new(ToolsetConfigSuite))
}
