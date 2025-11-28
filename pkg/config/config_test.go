package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

		toolsets = ["core", "config", "helm", "metrics"]
		
		enabled_tools = ["configuration_view", "events_list", "namespaces_list", "pods_list", "resources_list", "resources_get", "resources_create_or_update", "resources_delete"]
		disabled_tools = ["pods_delete", "pods_top", "pods_log", "pods_run", "pods_exec"]

		denied_resources = [
			{group = "apps", version = "v1", kind = "Deployment"},
			{group = "rbac.authorization.k8s.io", version = "v1", kind = "Role"}
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
			s.Contains(config.DeniedResources, GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
				"Expected denied resources to contain apps/v1/Deployment")
		})
		s.Run("contains rbac.authorization.k8s.io/v1/Role", func() {
			s.Contains(config.DeniedResources, GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "Role"},
				"Expected denied resources to contain rbac.authorization.k8s.io/v1/Role")
		})
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
	`), 0644)
	s.Require().NoError(err)

	config, err := Read(mainConfigPath, dropInDir)
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("overrides specified field", func() {
		s.True(config.ReadOnly, "read_only should be overridden to true")
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

func TestConfig(t *testing.T) {
	suite.Run(t, new(ConfigSuite))
}
