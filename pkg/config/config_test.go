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
	config, err := Read("non-existent-config.toml")
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

	config, err := Read(invalidConfigPath)
	s.Run("returns error for invalid file", func() {
		s.Require().NotNil(err, "Expected error for invalid file, got nil")
	})
	s.Run("error message contains toml error with line number", func() {
		expectedError := "toml: line 9"
		s.Truef(strings.HasPrefix(err.Error(), expectedError), "Expected error message to contain line number, got %v", err)
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

	config, err := Read(validConfigPath)
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
	validConfigPath := s.writeConfig(`
		port = "1337"
	`)

	config, err := Read(validConfigPath)
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

func TestConfig(t *testing.T) {
	suite.Run(t, new(ConfigSuite))
}
