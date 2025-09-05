package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadConfigMissingFile(t *testing.T) {
	config, err := ReadConfig("non-existent-config.toml")
	t.Run("returns error for missing file", func(t *testing.T) {
		if err == nil {
			t.Fatal("Expected error for missing file, got nil")
		}
		if config != nil {
			t.Fatalf("Expected nil config for missing file, got %v", config)
		}
	})
}

func TestReadConfigInvalid(t *testing.T) {
	invalidConfigPath := writeConfig(t, `
[[denied_resources]]
group = "apps"
version = "v1"
kind = "Deployment"
[[denied_resources]]
group = "rbac.authorization.k8s.io"
version = "v1"
kind = "Role
`)

	config, err := ReadConfig(invalidConfigPath)
	t.Run("returns error for invalid file", func(t *testing.T) {
		if err == nil {
			t.Fatal("Expected error for invalid file, got nil")
		}
		if config != nil {
			t.Fatalf("Expected nil config for invalid file, got %v", config)
		}
	})
	t.Run("error message contains toml error with line number", func(t *testing.T) {
		expectedError := "toml: line 9"
		if err != nil && !strings.HasPrefix(err.Error(), expectedError) {
			t.Fatalf("Expected error message '%s' to contain line number, got %v", expectedError, err)
		}
	})
}

func TestReadConfigValid(t *testing.T) {
	validConfigPath := writeConfig(t, `
log_level = 1
port = "9999"
sse_base_url = "https://example.com"
kubeconfig = "./path/to/config"
list_output = "yaml"
read_only = true
disable_destructive = true

denied_resources = [
    {group = "apps", version = "v1", kind = "Deployment"},
    {group = "rbac.authorization.k8s.io", version = "v1", kind = "Role"}
]

enabled_tools = ["configuration_view", "events_list", "namespaces_list", "pods_list", "resources_list", "resources_get", "resources_create_or_update", "resources_delete"]
disabled_tools = ["pods_delete", "pods_top", "pods_log", "pods_run", "pods_exec"]
`)

	config, err := ReadConfig(validConfigPath)
	t.Run("reads and unmarshalls file", func(t *testing.T) {
		if err != nil {
			t.Fatalf("ReadConfig returned an error for a valid file: %v", err)
		}
		if config == nil {
			t.Fatal("ReadConfig returned a nil config for a valid file")
		}
	})
	t.Run("denied resources are parsed correctly", func(t *testing.T) {
		if len(config.DeniedResources) != 2 {
			t.Fatalf("Expected 2 denied resources, got %d", len(config.DeniedResources))
		}
		if config.DeniedResources[0].Group != "apps" ||
			config.DeniedResources[0].Version != "v1" ||
			config.DeniedResources[0].Kind != "Deployment" {
			t.Errorf("Unexpected denied resources: %v", config.DeniedResources[0])
		}
	})
	t.Run("log_level parsed correctly", func(t *testing.T) {
		if config.LogLevel != 1 {
			t.Fatalf("Unexpected log level: %v", config.LogLevel)
		}
	})
	t.Run("port parsed correctly", func(t *testing.T) {
		if config.Port != "9999" {
			t.Fatalf("Unexpected port value: %v", config.Port)
		}
	})
	t.Run("sse_base_url parsed correctly", func(t *testing.T) {
		if config.SSEBaseURL != "https://example.com" {
			t.Fatalf("Unexpected sse_base_url value: %v", config.SSEBaseURL)
		}
	})
	t.Run("kubeconfig parsed correctly", func(t *testing.T) {
		if config.KubeConfig != "./path/to/config" {
			t.Fatalf("Unexpected kubeconfig value: %v", config.KubeConfig)
		}
	})
	t.Run("list_output parsed correctly", func(t *testing.T) {
		if config.ListOutput != "yaml" {
			t.Fatalf("Unexpected list_output value: %v", config.ListOutput)
		}
	})
	t.Run("read_only parsed correctly", func(t *testing.T) {
		if !config.ReadOnly {
			t.Fatalf("Unexpected read-only mode: %v", config.ReadOnly)
		}
	})
	t.Run("disable_destructive parsed correctly", func(t *testing.T) {
		if !config.DisableDestructive {
			t.Fatalf("Unexpected disable destructive: %v", config.DisableDestructive)
		}
	})
	t.Run("enabled_tools parsed correctly", func(t *testing.T) {
		if len(config.EnabledTools) != 8 {
			t.Fatalf("Unexpected enabled tools: %v", config.EnabledTools)

		}
		for i, tool := range []string{"configuration_view", "events_list", "namespaces_list", "pods_list", "resources_list", "resources_get", "resources_create_or_update", "resources_delete"} {
			if config.EnabledTools[i] != tool {
				t.Errorf("Expected enabled tool %d to be %s, got %s", i, tool, config.EnabledTools[i])
			}
		}
	})
	t.Run("disabled_tools parsed correctly", func(t *testing.T) {
		if len(config.DisabledTools) != 5 {
			t.Fatalf("Unexpected disabled tools: %v", config.DisabledTools)
		}
		for i, tool := range []string{"pods_delete", "pods_top", "pods_log", "pods_run", "pods_exec"} {
			if config.DisabledTools[i] != tool {
				t.Errorf("Expected disabled tool %d to be %s, got %s", i, tool, config.DisabledTools[i])
			}
		}
	})
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "config.toml")
	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file %s: %v", path, err)
	}
	return path
}
