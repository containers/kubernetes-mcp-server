package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
)

type BaseConfigSuite struct {
	suite.Suite
}

func (s *BaseConfigSuite) writeConfig(content string) string {
	s.T().Helper()
	tempDir := s.T().TempDir()
	path := filepath.Join(tempDir, "config.toml")
	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		s.T().Fatalf("Failed to write config file %s: %v", path, err)
	}
	return path
}

type ConfigSuite struct {
	BaseConfigSuite
}

func (s *ConfigSuite) TestReadConfigMissingFile() {
	config, err := Read("non-existent-config.toml", "")
	s.Run("returns error for missing file", func() {
		s.Require().NotNil(err, "Expected error for missing file, got nil")
		s.True(errors.Is(err, fs.ErrNotExist), "Expected ErrNotExist, got %v", err)
	})
	s.Run("returns nil config for missing file", func() {
		s.Nil(config, "Expected nil config for missing file")
	})
}

func (s *ConfigSuite) TestReadConfigInvalid() {
	invalidConfigPath := s.writeConfig(`
		[[denied_resources]]
		group = "apps"
		version = "v1"
		kind = "Deployment"
		[[denied_resources]]
		group = "rbac.authorization.k8s.io"
		version = "v1"
		kind = "Role
	`)

	config, err := Read(invalidConfigPath, "")
	s.Run("returns error for invalid file", func() {
		s.Require().NotNil(err, "Expected error for invalid file, got nil")
	})
	s.Run("error message contains toml error with line number", func() {
		expectedError := "toml: line 9"
		s.Truef(strings.Contains(err.Error(), expectedError), "Expected error message to contain line number, got %v", err)
	})
	s.Run("returns nil config for invalid file", func() {
		s.Nil(config, "Expected nil config for missing file")
	})
}

func (s *ConfigSuite) TestReadConfigValid() {
	validConfigPath := s.writeConfig(`
		log_level = 1
		port = "9999"
		sse_base_url = "https://example.com"
		kubeconfig = "./path/to/config"
		list_output = "yaml"
		read_only = true
		disable_destructive = true
		stateless = true

		toolsets = ["core", "config", "helm", "metrics"]
		
		enabled_tools = ["configuration_view", "events_list", "namespaces_list", "pods_list", "resources_list", "resources_get", "resources_create_or_update", "resources_delete"]
		disabled_tools = ["pods_delete", "pods_top", "pods_log", "pods_run", "pods_exec"]

		denied_resources = [
			{group = "apps", version = "v1", kind = "Deployment"},
			{group = "rbac.authorization.k8s.io", version = "v1", kind = "Role"}
		]

		[[prompts]]
		name = "k8s-troubleshoot"
		title = "Troubleshoot Kubernetes"
		description = "Troubleshoot common Kubernetes issues"
		arguments = [
			{name = "namespace", description = "Target namespace", required = true},
			{name = "resource", description = "Resource type to check", required = false}
		]
		messages = [
			{role = "user", content = "Check the health of resources in namespace {{namespace}}{{resource}}"}
		]

	`)

	config, err := Read(validConfigPath, "")
	s.Require().NotNil(config)
	s.Run("reads and unmarshalls file", func() {
		s.Nil(err, "Expected nil error for valid file")
		s.Require().NotNil(config, "Expected non-nil config for valid file")
	})
	s.Run("log_level parsed correctly", func() {
		s.Equalf(1, config.LogLevel, "Expected LogLevel to be 1, got %d", config.LogLevel)
	})
	s.Run("port parsed correctly", func() {
		s.Equalf("9999", config.Port, "Expected Port to be 9999, got %s", config.Port)
	})
	s.Run("sse_base_url parsed correctly", func() {
		s.Equalf("https://example.com", config.SSEBaseURL, "Expected SSEBaseURL to be https://example.com, got %s", config.SSEBaseURL)
	})
	s.Run("kubeconfig parsed correctly", func() {
		s.Equalf("./path/to/config", config.KubeConfig, "Expected KubeConfig to be ./path/to/config, got %s", config.KubeConfig)
	})
	s.Run("list_output parsed correctly", func() {
		s.Equalf("yaml", config.ListOutput, "Expected ListOutput to be yaml, got %s", config.ListOutput)
	})
	s.Run("read_only parsed correctly", func() {
		s.Truef(config.ReadOnly, "Expected ReadOnly to be true, got %v", config.ReadOnly)
	})
	s.Run("disable_destructive parsed correctly", func() {
		s.Truef(config.DisableDestructive, "Expected DisableDestructive to be true, got %v", config.DisableDestructive)
	})
	s.Run("stateless parsed correctly", func() {
		s.Truef(config.Stateless, "Expected Stateless to be true, got %v", config.Stateless)
	})
	s.Run("toolsets", func() {
		s.Require().Lenf(config.Toolsets, 4, "Expected 4 toolsets, got %d", len(config.Toolsets))
		for _, toolset := range []string{"core", "config", "helm", "metrics"} {
			s.Containsf(config.Toolsets, toolset, "Expected toolsets to contain %s", toolset)
		}
	})
	s.Run("enabled_tools", func() {
		s.Require().Lenf(config.EnabledTools, 8, "Expected 8 enabled tools, got %d", len(config.EnabledTools))
		for _, tool := range []string{"configuration_view", "events_list", "namespaces_list", "pods_list", "resources_list", "resources_get", "resources_create_or_update", "resources_delete"} {
			s.Containsf(config.EnabledTools, tool, "Expected enabled tools to contain %s", tool)
		}
	})
	s.Run("disabled_tools", func() {
		s.Require().Lenf(config.DisabledTools, 5, "Expected 5 disabled tools, got %d", len(config.DisabledTools))
		for _, tool := range []string{"pods_delete", "pods_top", "pods_log", "pods_run", "pods_exec"} {
			s.Containsf(config.DisabledTools, tool, "Expected disabled tools to contain %s", tool)
		}
	})
	s.Run("denied_resources", func() {
		s.Require().Lenf(config.DeniedResources, 2, "Expected 2 denied resources, got %d", len(config.DeniedResources))
		s.Run("contains apps/v1/Deployment", func() {
			s.Contains(config.DeniedResources, api.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
				"Expected denied resources to contain apps/v1/Deployment")
		})
		s.Run("contains rbac.authorization.k8s.io/v1/Role", func() {
			s.Contains(config.DeniedResources, api.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "Role"},
				"Expected denied resources to contain rbac.authorization.k8s.io/v1/Role")
		})
	})
	s.Run("prompts", func() {
		s.Require().Lenf(config.Prompts, 1, "Expected 1 prompt, got %d", len(config.Prompts))
		prompt := config.Prompts[0]
		s.Run("name parsed correctly", func() {
			s.Equal("k8s-troubleshoot", prompt.Name)
		})
		s.Run("title parsed correctly", func() {
			s.Equal("Troubleshoot Kubernetes", prompt.Title)
		})
		s.Run("description parsed correctly", func() {
			s.Equal("Troubleshoot common Kubernetes issues", prompt.Description)
		})
		s.Run("arguments parsed correctly", func() {
			s.Require().Len(prompt.Arguments, 2)
			s.Equal("namespace", prompt.Arguments[0].Name)
			s.Equal("Target namespace", prompt.Arguments[0].Description)
			s.True(prompt.Arguments[0].Required)
			s.Equal("resource", prompt.Arguments[1].Name)
			s.Equal("Resource type to check", prompt.Arguments[1].Description)
			s.False(prompt.Arguments[1].Required)
		})
		s.Run("messages parsed correctly", func() {
			s.Require().Len(prompt.Templates, 1)
			s.Equal("user", prompt.Templates[0].Role)
			s.Equal("Check the health of resources in namespace {{namespace}}{{resource}}", prompt.Templates[0].Content)
		})
	})
}

func (s *ConfigSuite) TestReadConfigStatelessDefaults() {
	// Test that stateless defaults to false when not specified
	configPath := s.writeConfig(`
		log_level = 1
		port = "8080"
	`)

	config, err := Read(configPath, "")
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("stateless defaults to false", func() {
		s.Falsef(config.Stateless, "Expected Stateless to default to false, got %v", config.Stateless)
	})
}

func (s *ConfigSuite) TestReadConfigStatelessExplicitFalse() {
	// Test that stateless can be explicitly set to false
	configPath := s.writeConfig(`
		log_level = 1
		port = "8080"
		stateless = false
	`)

	config, err := Read(configPath, "")
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("stateless explicit false", func() {
		s.Falsef(config.Stateless, "Expected Stateless to be false, got %v", config.Stateless)
	})
}

func (s *ConfigSuite) TestReadConfigValidPreservesDefaultsForMissingFields() {
	if HasDefaultOverrides() {
		s.T().Skip("Skipping test because default configuration overrides are present (this is a downstream fork)")
	}
	validConfigPath := s.writeConfig(`
		port = "1337"
	`)

	config, err := Read(validConfigPath, "")
	s.Require().NotNil(config)
	s.Run("reads and unmarshalls file", func() {
		s.Nil(err, "Expected nil error for valid file")
		s.Require().NotNil(config, "Expected non-nil config for valid file")
	})
	s.Run("log_level defaulted correctly", func() {
		s.Equalf(0, config.LogLevel, "Expected LogLevel to be 0, got %d", config.LogLevel)
	})
	s.Run("port parsed correctly", func() {
		s.Equalf("1337", config.Port, "Expected Port to be 9999, got %s", config.Port)
	})
	s.Run("list_output defaulted correctly", func() {
		s.Equalf("table", config.ListOutput, "Expected ListOutput to be table, got %s", config.ListOutput)
	})
	s.Run("toolsets defaulted correctly", func() {
		s.Require().Lenf(config.Toolsets, 3, "Expected 3 toolsets, got %d", len(config.Toolsets))
		for _, toolset := range []string{"core", "config", "helm"} {
			s.Containsf(config.Toolsets, toolset, "Expected toolsets to contain %s", toolset)
		}
	})
}

func (s *ConfigSuite) TestGetSortedConfigFiles() {
	tempDir := s.T().TempDir()

	// Create test files
	files := []string{
		"10-first.toml",
		"20-second.toml",
		"05-before.toml",
		"99-last.toml",
		".hidden.toml", // should be ignored
		"readme.txt",   // should be ignored
		"invalid",      // should be ignored
	}

	for _, file := range files {
		path := filepath.Join(tempDir, file)
		err := os.WriteFile(path, []byte(""), 0644)
		s.Require().NoError(err)
	}

	// Create a subdirectory (should be ignored)
	subDir := filepath.Join(tempDir, "subdir")
	err := os.Mkdir(subDir, 0755)
	s.Require().NoError(err)

	sorted, err := getSortedConfigFiles(tempDir)
	s.Require().NoError(err)

	s.Run("returns only .toml files", func() {
		s.Len(sorted, 4, "Expected 4 .toml files")
	})

	s.Run("sorted in lexical order", func() {
		expected := []string{
			filepath.Join(tempDir, "05-before.toml"),
			filepath.Join(tempDir, "10-first.toml"),
			filepath.Join(tempDir, "20-second.toml"),
			filepath.Join(tempDir, "99-last.toml"),
		}
		s.Equal(expected, sorted)
	})

	s.Run("excludes dotfiles", func() {
		for _, file := range sorted {
			s.NotContains(file, ".hidden")
		}
	})

	s.Run("excludes non-.toml files", func() {
		for _, file := range sorted {
			s.Contains(file, ".toml")
		}
	})
}

func (s *ConfigSuite) TestDropInConfigPrecedence() {
	tempDir := s.T().TempDir()

	// Main config file
	mainConfigPath := s.writeConfig(`
		log_level = 1
		port = "8080"
		list_output = "table"
		toolsets = ["core", "config"]
	`)

	// Create drop-in directory
	dropInDir := filepath.Join(tempDir, "config.d")
	err := os.Mkdir(dropInDir, 0755)
	s.Require().NoError(err)

	// First drop-in file
	dropIn1 := filepath.Join(dropInDir, "10-override.toml")
	err = os.WriteFile(dropIn1, []byte(`
		log_level = 5
		port = "9090"
	`), 0644)
	s.Require().NoError(err)

	// Second drop-in file (should override first)
	dropIn2 := filepath.Join(dropInDir, "20-final.toml")
	err = os.WriteFile(dropIn2, []byte(`
		port = "7777"
		list_output = "yaml"
	`), 0644)
	s.Require().NoError(err)

	config, err := Read(mainConfigPath, dropInDir)
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("drop-in overrides main config", func() {
		s.Equal(5, config.LogLevel, "log_level from 10-override.toml should override main")
	})

	s.Run("later drop-in overrides earlier drop-in", func() {
		s.Equal("7777", config.Port, "port from 20-final.toml should override 10-override.toml")
	})

	s.Run("preserves values not in drop-in files", func() {
		s.Equal([]string{"core", "config"}, config.Toolsets, "toolsets from main config should be preserved")
	})

	s.Run("applies all drop-in changes", func() {
		s.Equal("yaml", config.ListOutput, "list_output from 20-final.toml should be applied")
	})
}

func (s *ConfigSuite) TestDropInConfigMissingDirectory() {
	mainConfigPath := s.writeConfig(`
		log_level = 3
		port = "8080"
	`)

	config, err := Read(mainConfigPath, "/non/existent/directory")
	s.Require().NoError(err, "Should not error for missing drop-in directory")
	s.Require().NotNil(config)

	s.Run("loads main config successfully", func() {
		s.Equal(3, config.LogLevel)
		s.Equal("8080", config.Port)
	})
}

func (s *ConfigSuite) TestDropInConfigEmptyDirectory() {
	mainConfigPath := s.writeConfig(`
		log_level = 2
	`)

	dropInDir := s.T().TempDir()

	config, err := Read(mainConfigPath, dropInDir)
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("loads main config successfully", func() {
		s.Equal(2, config.LogLevel)
	})
}

func (s *ConfigSuite) TestDropInConfigPartialOverride() {
	tempDir := s.T().TempDir()

	mainConfigPath := s.writeConfig(`
		log_level = 1
		port = "8080"
		list_output = "table"
		read_only = false
		toolsets = ["core", "config", "helm"]
	`)

	dropInDir := filepath.Join(tempDir, "config.d")
	err := os.Mkdir(dropInDir, 0755)
	s.Require().NoError(err)

	// Drop-in file with partial config
	dropIn := filepath.Join(dropInDir, "10-partial.toml")
	err = os.WriteFile(dropIn, []byte(`
		read_only = true
		stateless = true
	`), 0644)
	s.Require().NoError(err)

	config, err := Read(mainConfigPath, dropInDir)
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("overrides specified field", func() {
		s.True(config.ReadOnly, "read_only should be overridden to true")
		s.True(config.Stateless, "stateless should be overridden to true")
	})

	s.Run("preserves all other fields", func() {
		s.Equal(1, config.LogLevel)
		s.Equal("8080", config.Port)
		s.Equal("table", config.ListOutput)
		s.Equal([]string{"core", "config", "helm"}, config.Toolsets)
	})
}

func (s *ConfigSuite) TestDropInConfigWithArrays() {
	tempDir := s.T().TempDir()

	mainConfigPath := s.writeConfig(`
		toolsets = ["core", "config"]
		enabled_tools = ["tool1", "tool2"]
	`)

	dropInDir := filepath.Join(tempDir, "config.d")
	err := os.Mkdir(dropInDir, 0755)
	s.Require().NoError(err)

	dropIn := filepath.Join(dropInDir, "10-arrays.toml")
	err = os.WriteFile(dropIn, []byte(`
		toolsets = ["helm", "logs"]
	`), 0644)
	s.Require().NoError(err)

	config, err := Read(mainConfigPath, dropInDir)
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("replaces arrays completely", func() {
		s.Equal([]string{"helm", "logs"}, config.Toolsets, "toolsets should be completely replaced")
		s.Equal([]string{"tool1", "tool2"}, config.EnabledTools, "enabled_tools should be preserved")
	})
}

func (s *ConfigSuite) TestDefaultConfDResolution() {
	// Create a temp directory structure:
	// tempDir/
	//   config.toml
	//   conf.d/
	//     10-override.toml
	tempDir := s.T().TempDir()

	// Create main config file
	mainConfigPath := filepath.Join(tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(mainConfigPath, []byte(`
		log_level = 1
		port = "8080"
	`), 0644))

	// Create default conf.d directory
	confDDir := filepath.Join(tempDir, "conf.d")
	s.Require().NoError(os.Mkdir(confDDir, 0755))

	// Create drop-in file in conf.d
	dropIn := filepath.Join(confDDir, "10-override.toml")
	s.Require().NoError(os.WriteFile(dropIn, []byte(`
		log_level = 5
		port = "9090"
	`), 0644))

	// Read config WITHOUT specifying drop-in directory - should auto-discover conf.d
	config, err := Read(mainConfigPath, "")
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("auto-discovers conf.d relative to config file", func() {
		s.Equal(5, config.LogLevel, "log_level should be overridden by conf.d/10-override.toml")
		s.Equal("9090", config.Port, "port should be overridden by conf.d/10-override.toml")
	})
}

func (s *ConfigSuite) TestDefaultConfDNotExist() {
	// When conf.d doesn't exist, config should still load without error
	mainConfigPath := s.writeConfig(`
		log_level = 3
		port = "8080"
	`)

	config, err := Read(mainConfigPath, "")
	s.Require().NoError(err, "Should not error when default conf.d doesn't exist")
	s.Require().NotNil(config)

	s.Run("loads main config when conf.d doesn't exist", func() {
		s.Equal(3, config.LogLevel)
		s.Equal("8080", config.Port)
	})
}

func (s *ConfigSuite) TestStandaloneConfigDir() {
	// Test using only --config-dir without --config (standalone mode)
	tempDir := s.T().TempDir()

	// Create first drop-in file
	dropIn1 := filepath.Join(tempDir, "10-base.toml")
	s.Require().NoError(os.WriteFile(dropIn1, []byte(`
		log_level = 2
		port = "8080"
		toolsets = ["core"]
	`), 0644))

	// Create second drop-in file
	dropIn2 := filepath.Join(tempDir, "20-override.toml")
	s.Require().NoError(os.WriteFile(dropIn2, []byte(`
		log_level = 5
		list_output = "yaml"
	`), 0644))

	// Read with empty config path (standalone --config-dir)
	config, err := Read("", tempDir)
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("loads config from drop-in directory only", func() {
		s.Equal(5, config.LogLevel, "log_level should be from 20-override.toml")
		s.Equal("8080", config.Port, "port should be from 10-base.toml")
		s.Equal("yaml", config.ListOutput, "list_output should be from 20-override.toml")
		s.Equal([]string{"core"}, config.Toolsets, "toolsets should be from 10-base.toml")
	})
}

func (s *ConfigSuite) TestStandaloneConfigDirPreservesDefaults() {
	// Test that defaults are preserved when using standalone --config-dir
	if HasDefaultOverrides() {
		s.T().Skip("Skipping test because default configuration overrides are present (this is a downstream fork)")
	}
	tempDir := s.T().TempDir()

	// Create a drop-in file with only partial config
	dropIn := filepath.Join(tempDir, "10-partial.toml")
	s.Require().NoError(os.WriteFile(dropIn, []byte(`
		port = "9999"
	`), 0644))

	config, err := Read("", tempDir)
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("preserves default values", func() {
		s.Equal("9999", config.Port, "port should be from drop-in")
		s.Equal("table", config.ListOutput, "list_output should be default")
		s.Equal([]string{"core", "config", "helm"}, config.Toolsets, "toolsets should be default")
	})
}

func (s *ConfigSuite) TestStandaloneConfigDirEmpty() {
	// Test standalone --config-dir with empty directory
	if HasDefaultOverrides() {
		s.T().Skip("Skipping test because default configuration overrides are present (this is a downstream fork)")
	}
	tempDir := s.T().TempDir()

	config, err := Read("", tempDir)
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("returns defaults for empty directory", func() {
		s.Equal("table", config.ListOutput, "list_output should be default")
		s.Equal([]string{"core", "config", "helm"}, config.Toolsets, "toolsets should be default")
	})
}

func (s *ConfigSuite) TestStandaloneConfigDirNonExistent() {
	// Test standalone --config-dir with non-existent directory
	if HasDefaultOverrides() {
		s.T().Skip("Skipping test because default configuration overrides are present (this is a downstream fork)")
	}
	config, err := Read("", "/non/existent/directory")
	s.Require().NoError(err, "Should not error for non-existent directory")
	s.Require().NotNil(config)

	s.Run("returns defaults for non-existent directory", func() {
		s.Equal("table", config.ListOutput, "list_output should be default")
	})
}

func (s *ConfigSuite) TestConfigDirOverridesDefaultConfD() {
	// Test that explicit --config-dir overrides default conf.d
	tempDir := s.T().TempDir()

	// Create main config file
	mainConfigPath := filepath.Join(tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(mainConfigPath, []byte(`
		log_level = 1
		port = "8080"
	`), 0644))

	// Create default conf.d directory with a drop-in
	confDDir := filepath.Join(tempDir, "conf.d")
	s.Require().NoError(os.Mkdir(confDDir, 0755))
	s.Require().NoError(os.WriteFile(filepath.Join(confDDir, "10-default.toml"), []byte(`
		log_level = 99
		port = "1111"
	`), 0644))

	// Create custom drop-in directory
	customDir := filepath.Join(tempDir, "custom.d")
	s.Require().NoError(os.Mkdir(customDir, 0755))
	s.Require().NoError(os.WriteFile(filepath.Join(customDir, "10-custom.toml"), []byte(`
		log_level = 5
		port = "9090"
	`), 0644))

	// Read with explicit config-dir (should override default conf.d)
	config, err := Read(mainConfigPath, customDir)
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("uses explicit config-dir instead of default conf.d", func() {
		s.Equal(5, config.LogLevel, "log_level should be from custom.d, not conf.d")
		s.Equal("9090", config.Port, "port should be from custom.d, not conf.d")
	})
}

func (s *ConfigSuite) TestInvalidTomlInDropIn() {
	tempDir := s.T().TempDir()

	mainConfigPath := s.writeConfig(`
		log_level = 1
	`)

	// Create drop-in directory with invalid TOML
	dropInDir := filepath.Join(tempDir, "config.d")
	s.Require().NoError(os.Mkdir(dropInDir, 0755))

	// Valid first file
	s.Require().NoError(os.WriteFile(filepath.Join(dropInDir, "10-valid.toml"), []byte(`
		port = "8080"
	`), 0644))

	// Invalid second file
	s.Require().NoError(os.WriteFile(filepath.Join(dropInDir, "20-invalid.toml"), []byte(`
		port = "unclosed string
	`), 0644))

	config, err := Read(mainConfigPath, dropInDir)

	s.Run("returns error for invalid TOML in drop-in", func() {
		s.Require().Error(err, "Expected error for invalid TOML")
		s.Contains(err.Error(), "20-invalid.toml", "Error should mention the invalid file")
	})

	s.Run("returns nil config", func() {
		s.Nil(config, "Expected nil config when drop-in has invalid TOML")
	})
}

func (s *ConfigSuite) TestAbsoluteDropInConfigDir() {
	// Test that absolute paths work for --config-dir
	tempDir := s.T().TempDir()

	// Create main config in one directory
	configDir := filepath.Join(tempDir, "config")
	s.Require().NoError(os.Mkdir(configDir, 0755))
	mainConfigPath := filepath.Join(configDir, "config.toml")
	s.Require().NoError(os.WriteFile(mainConfigPath, []byte(`
		log_level = 1
	`), 0644))

	// Create drop-in directory in a completely different location
	dropInDir := filepath.Join(tempDir, "somewhere", "else", "conf.d")
	s.Require().NoError(os.MkdirAll(dropInDir, 0755))
	s.Require().NoError(os.WriteFile(filepath.Join(dropInDir, "10-override.toml"), []byte(`
		log_level = 9
		port = "7777"
	`), 0644))

	// Use absolute path for config-dir
	absDropInDir, err := filepath.Abs(dropInDir)
	s.Require().NoError(err)

	config, err := Read(mainConfigPath, absDropInDir)
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("loads from absolute drop-in path", func() {
		s.Equal(9, config.LogLevel, "log_level should be from absolute path drop-in")
		s.Equal("7777", config.Port, "port should be from absolute path drop-in")
	})
}

func (s *ConfigSuite) TestDropInNotADirectory() {
	tempDir := s.T().TempDir()

	mainConfigPath := s.writeConfig(`
		log_level = 1
	`)

	// Create a file (not a directory) where drop-in dir is expected
	notADir := filepath.Join(tempDir, "not-a-dir")
	s.Require().NoError(os.WriteFile(notADir, []byte("i am a file"), 0644))

	config, err := Read(mainConfigPath, notADir)

	s.Run("returns error when drop-in path is not a directory", func() {
		s.Require().Error(err)
		s.Contains(err.Error(), "not a directory")
	})

	s.Run("returns nil config", func() {
		s.Nil(config)
	})
}

func (s *ConfigSuite) TestDeepMerge() {
	s.Run("merges flat maps", func() {
		dst := map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		}
		src := map[string]interface{}{
			"key2": "overridden",
			"key3": "value3",
		}

		deepMerge(dst, src)

		s.Equal("value1", dst["key1"], "existing key should be preserved")
		s.Equal("overridden", dst["key2"], "overlapping key should be overridden")
		s.Equal("value3", dst["key3"], "new key should be added")
	})

	s.Run("recursively merges nested maps", func() {
		dst := map[string]interface{}{
			"nested": map[string]interface{}{
				"a": "original-a",
				"b": "original-b",
			},
		}
		src := map[string]interface{}{
			"nested": map[string]interface{}{
				"b": "overridden-b",
				"c": "new-c",
			},
		}

		deepMerge(dst, src)

		nested := dst["nested"].(map[string]interface{})
		s.Equal("original-a", nested["a"], "nested key not in src should be preserved")
		s.Equal("overridden-b", nested["b"], "nested key in both should be overridden")
		s.Equal("new-c", nested["c"], "new nested key should be added")
	})

	s.Run("overwrites when types differ", func() {
		dst := map[string]interface{}{
			"key": map[string]interface{}{"nested": "value"},
		}
		src := map[string]interface{}{
			"key": "now-a-string",
		}

		deepMerge(dst, src)

		s.Equal("now-a-string", dst["key"], "map should be replaced by string")
	})

	s.Run("replaces arrays completely", func() {
		dst := map[string]interface{}{
			"array": []interface{}{"a", "b", "c"},
		}
		src := map[string]interface{}{
			"array": []interface{}{"x", "y"},
		}

		deepMerge(dst, src)

		s.Equal([]interface{}{"x", "y"}, dst["array"], "arrays should be replaced, not merged")
	})

	s.Run("deeply nested merge", func() {
		dst := map[string]interface{}{
			"level1": map[string]interface{}{
				"level2": map[string]interface{}{
					"level3": map[string]interface{}{
						"deep": "original",
						"keep": "preserved",
					},
				},
			},
		}
		src := map[string]interface{}{
			"level1": map[string]interface{}{
				"level2": map[string]interface{}{
					"level3": map[string]interface{}{
						"deep": "overridden",
					},
				},
			},
		}

		deepMerge(dst, src)

		level3 := dst["level1"].(map[string]interface{})["level2"].(map[string]interface{})["level3"].(map[string]interface{})
		s.Equal("overridden", level3["deep"], "deeply nested key should be overridden")
		s.Equal("preserved", level3["keep"], "deeply nested key not in src should be preserved")
	})
}

func (s *ConfigSuite) TestDropInWithDeniedResources() {
	tempDir := s.T().TempDir()

	// Main config with some denied resources
	mainConfigPath := filepath.Join(tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(mainConfigPath, []byte(`
		log_level = 1
		denied_resources = [
			{group = "apps", version = "v1", kind = "Deployment"}
		]
	`), 0644))

	// Create drop-in directory
	dropInDir := filepath.Join(tempDir, "conf.d")
	s.Require().NoError(os.Mkdir(dropInDir, 0755))

	// Drop-in that replaces denied_resources (arrays are replaced, not merged)
	s.Require().NoError(os.WriteFile(filepath.Join(dropInDir, "10-security.toml"), []byte(`
		denied_resources = [
			{group = "rbac.authorization.k8s.io", version = "v1", kind = "ClusterRole"},
			{group = "rbac.authorization.k8s.io", version = "v1", kind = "ClusterRoleBinding"}
		]
	`), 0644))

	config, err := Read(mainConfigPath, "")
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("drop-in replaces denied_resources array", func() {
		s.Len(config.DeniedResources, 2, "denied_resources should have 2 entries from drop-in")
		s.Contains(config.DeniedResources, api.GroupVersionKind{
			Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole",
		})
		s.Contains(config.DeniedResources, api.GroupVersionKind{
			Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRoleBinding",
		})
	})

	s.Run("original denied_resources from main config are replaced", func() {
		s.NotContains(config.DeniedResources, api.GroupVersionKind{
			Group: "apps", Version: "v1", Kind: "Deployment",
		}, "original entry should be replaced by drop-in")
	})
}

func (s *ConfigSuite) TestRelativeConfigDirPath() {
	// Test that relative --config-dir paths are resolved relative to --config file
	tempDir := s.T().TempDir()

	// Create main config in a subdirectory
	configSubDir := filepath.Join(tempDir, "etc", "kmcp")
	s.Require().NoError(os.MkdirAll(configSubDir, 0755))
	mainConfigPath := filepath.Join(configSubDir, "config.toml")
	s.Require().NoError(os.WriteFile(mainConfigPath, []byte(`
		log_level = 1
	`), 0644))

	// Create a custom drop-in dir relative to config (sibling directory)
	customDropInDir := filepath.Join(configSubDir, "overrides.d")
	s.Require().NoError(os.Mkdir(customDropInDir, 0755))
	s.Require().NoError(os.WriteFile(filepath.Join(customDropInDir, "10-override.toml"), []byte(`
		log_level = 7
		port = "3333"
	`), 0644))

	// Use relative path for config-dir
	config, err := Read(mainConfigPath, "overrides.d")
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("resolves relative config-dir against config file directory", func() {
		s.Equal(7, config.LogLevel, "log_level should be from overrides.d")
		s.Equal("3333", config.Port, "port should be from overrides.d")
	})
}

func (s *ConfigSuite) TestBothConfigAndConfigDirEmpty() {
	// Edge case: Read("", "") should return defaults
	if HasDefaultOverrides() {
		s.T().Skip("Skipping test because default configuration overrides are present (this is a downstream fork)")
	}

	config, err := Read("", "")
	s.Require().NoError(err, "Should not error when both config and config-dir are empty")
	s.Require().NotNil(config)

	s.Run("returns default configuration", func() {
		s.Equal("table", config.ListOutput)
		s.Equal([]string{"core", "config", "helm"}, config.Toolsets)
		s.Equal(0, config.LogLevel)
	})
}

func (s *ConfigSuite) TestMultipleDropInFilesInOrder() {
	// Comprehensive test of file ordering with many files
	tempDir := s.T().TempDir()

	// Create main config
	mainConfigPath := filepath.Join(tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(mainConfigPath, []byte(`
		log_level = 0
		port = "initial"
		list_output = "table"
	`), 0644))

	// Create conf.d with multiple files
	confDDir := filepath.Join(tempDir, "conf.d")
	s.Require().NoError(os.Mkdir(confDDir, 0755))

	// Create files in non-alphabetical order to ensure sorting works
	files := map[string]string{
		"50-middle.toml": `port = "fifty"`,
		"10-first.toml":  `log_level = 10`,
		"90-last.toml":   `log_level = 90`,
		"30-third.toml":  `list_output = "yaml"`,
		"70-seven.toml":  `port = "seventy"`,
	}

	for name, content := range files {
		s.Require().NoError(os.WriteFile(filepath.Join(confDDir, name), []byte(content), 0644))
	}

	config, err := Read(mainConfigPath, "")
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("processes files in lexical order", func() {
		// log_level: main(0) -> 10-first(10) -> 90-last(90) = 90
		s.Equal(90, config.LogLevel, "log_level should be from 90-last.toml (last to set it)")
	})

	s.Run("last file wins for each field", func() {
		// port: main("initial") -> 50-middle("fifty") -> 70-seven("seventy") = "seventy"
		s.Equal("seventy", config.Port, "port should be from 70-seven.toml (last to set it)")
		// list_output: main("table") -> 30-third("yaml") = "yaml"
		s.Equal("yaml", config.ListOutput, "list_output should be from 30-third.toml")
	})
}

func (s *ConfigSuite) TestDropInWithNestedConfig() {
	// Test that nested config structures (like cluster_provider_configs) merge correctly
	tempDir := s.T().TempDir()

	mainConfigPath := filepath.Join(tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(mainConfigPath, []byte(`
		log_level = 1
		cluster_provider_strategy = "kubeconfig"

		[cluster_provider_configs.kubeconfig]
		setting1 = "from-main"
		setting2 = "from-main"
	`), 0644))

	confDDir := filepath.Join(tempDir, "conf.d")
	s.Require().NoError(os.Mkdir(confDDir, 0755))

	// First drop-in overrides one nested setting
	s.Require().NoError(os.WriteFile(filepath.Join(confDDir, "10-partial.toml"), []byte(`
		[cluster_provider_configs.kubeconfig]
		setting1 = "from-drop-in-1"
	`), 0644))

	// Second drop-in overrides another nested setting
	s.Require().NoError(os.WriteFile(filepath.Join(confDDir, "20-partial.toml"), []byte(`
		[cluster_provider_configs.kubeconfig]
		setting2 = "from-drop-in-2"
		setting3 = "new-in-drop-in-2"
	`), 0644))

	config, err := Read(mainConfigPath, "")
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("merges nested config from multiple drop-ins", func() {
		// The raw ClusterProviderConfigs should have all merged keys
		s.NotNil(config.ClusterProviderConfigs["kubeconfig"])
	})
}

func (s *ConfigSuite) TestEmptyConfigFile() {
	// Test that an empty main config file works correctly
	if HasDefaultOverrides() {
		s.T().Skip("Skipping test because default configuration overrides are present (this is a downstream fork)")
	}

	tempDir := s.T().TempDir()

	// Create empty main config
	mainConfigPath := filepath.Join(tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(mainConfigPath, []byte(``), 0644))

	// Create conf.d with overrides
	confDDir := filepath.Join(tempDir, "conf.d")
	s.Require().NoError(os.Mkdir(confDDir, 0755))
	s.Require().NoError(os.WriteFile(filepath.Join(confDDir, "10-settings.toml"), []byte(`
		log_level = 5
		port = "9999"
	`), 0644))

	config, err := Read(mainConfigPath, "")
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("applies drop-in on top of defaults when main config is empty", func() {
		s.Equal(5, config.LogLevel, "log_level should be from drop-in")
		s.Equal("9999", config.Port, "port should be from drop-in")
		// Defaults should still be applied for unset values
		s.Equal("table", config.ListOutput, "list_output should be default")
		s.Equal([]string{"core", "config", "helm"}, config.Toolsets, "toolsets should be default")
	})
}

func TestConfig(t *testing.T) {
	suite.Run(t, new(ConfigSuite))
}
