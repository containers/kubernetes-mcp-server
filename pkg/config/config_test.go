package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
)

type ConfigFileSuite struct {
	suite.Suite
}

func (s *ConfigFileSuite) writeConfig(content string) string {
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
	ConfigFileSuite
	defaults *Config
}

func (s *ConfigSuite) SetupTest() {
	s.defaults = New()
}

func (s *ConfigSuite) TestBaseDefaultValues() {
	base := BaseDefault()
	s.Run("ListOutput is table", func() {
		s.Equal("table", base.ListOutput.Get())
	})
	s.Run("Toolsets are core, config", func() {
		s.Equal([]string{"core", "config"}, base.Toolsets.Get())
	})
	s.Run("ReadOnly is false", func() {
		s.False(base.ReadOnly.Get())
	})
	s.Run("DisableDestructive is false", func() {
		s.False(base.DisableDestructive.Get())
	})
	s.Run("Stateless is false", func() {
		s.False(base.Stateless.Get())
	})
	s.Run("LogLevel is 0", func() {
		s.Equal(0, base.LogLevel.Get())
	})
}

func (s *ConfigSuite) TestDocumentedOptions() {
	opts := DocumentedOptions()
	s.Require().NotEmpty(opts)
	paths := make(map[string]DocumentedOption, len(opts))
	for _, o := range opts {
		s.NotEmpty(o.Path, "documented option must have a TOML path")
		s.NotEmpty(o.Description, "documented option %s must have a description", o.Path)
		_, dup := paths[o.Path]
		s.False(dup, "duplicate documented path %s", o.Path)
		paths[o.Path] = o
	}
	s.Contains(paths, "port")
	s.Contains(paths, "http.rate_limit_burst")
	s.Contains(paths, "telemetry.endpoint")
	s.Equal("OTEL_EXPORTER_OTLP_ENDPOINT", paths["telemetry.endpoint"].EnvName)
	s.False(paths["port"].Reloadable)
	s.True(paths["log_level"].Reloadable)
	s.True(paths["token_exchange.client_auth.client_secret"].Sensitive)
}

func (s *ConfigSuite) TestReadConfigMissingFile() {
	config, err := Read(s.T().Context(), "non-existent-config.toml", "")
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

	config, err := Read(s.T().Context(), invalidConfigPath, "")
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
	tmpDir := s.T().TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")
	s.Require().NoError(os.WriteFile(certPath, []byte("test"), 0o644))
	s.Require().NoError(os.WriteFile(keyPath, []byte("test"), 0o644))

	validConfigPath := s.writeConfig(fmt.Sprintf(`
		log_level = 1
		port = "9999"
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

		# TLS configuration
		tls_cert = %q
		tls_key = %q

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

	`, certPath, keyPath))

	cfg, err := Read(s.T().Context(), validConfigPath, "")
	s.Require().NoError(err)
	s.Require().NotNil(cfg)

	s.Run("scalars", func() {
		cases := []struct {
			name string
			got  any
			want any
		}{
			{"log_level", cfg.LogLevel.Get(), 1},
			{"port", cfg.Port.Get(), "9999"},
			{"kubeconfig", cfg.KubeConfig.Get(), "./path/to/config"},
			{"list_output", cfg.ListOutput.Get(), "yaml"},
			{"read_only", cfg.ReadOnly.Get(), true},
			{"disable_destructive", cfg.DisableDestructive.Get(), true},
			{"stateless", cfg.Stateless.Get(), true},
			{"tls_cert", cfg.TLSCert.Get(), certPath},
			{"tls_key", cfg.TLSKey.Get(), keyPath},
		}
		for _, tc := range cases {
			s.Run(tc.name, func() {
				s.Equal(tc.want, tc.got)
			})
		}
	})

	s.Run("slices", func() {
		cases := []struct {
			name string
			got  []string
			want []string
		}{
			{"toolsets", cfg.Toolsets.Get(), []string{"core", "config", "helm", "metrics"}},
			{"enabled_tools", cfg.EnabledTools.Get(), []string{"configuration_view", "events_list", "namespaces_list", "pods_list", "resources_list", "resources_get", "resources_create_or_update", "resources_delete"}},
			{"disabled_tools", cfg.DisabledTools.Get(), []string{"pods_delete", "pods_top", "pods_log", "pods_run", "pods_exec"}},
		}
		for _, tc := range cases {
			s.Run(tc.name, func() {
				s.Equal(tc.want, tc.got)
			})
		}
	})

	s.Run("denied_resources", func() {
		s.Equal([]api.GroupVersionKind{
			{Group: "apps", Version: "v1", Kind: "Deployment"},
			{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "Role"},
		}, cfg.DeniedResources.Get())
	})

	s.Run("prompts", func() {
		s.Require().Len(cfg.Prompts.Get(), 1)
		prompt := cfg.Prompts.Get()[0]
		s.Equal("k8s-troubleshoot", prompt.Name)
		s.Equal("Troubleshoot Kubernetes", prompt.Title)
		s.Equal("Troubleshoot common Kubernetes issues", prompt.Description)
		s.Equal([]api.PromptArgument{
			{Name: "namespace", Description: "Target namespace", Required: true},
			{Name: "resource", Description: "Resource type to check", Required: false},
		}, prompt.Arguments)
		s.Require().Len(prompt.Templates, 1)
		s.Equal("user", prompt.Templates[0].Role)
		s.Equal("Check the health of resources in namespace {{namespace}}{{resource}}", prompt.Templates[0].Content)
	})
}

func (s *ConfigSuite) TestReadConfigStatelessDefaults() {
	// Test that stateless defaults to false when not specified
	configPath := s.writeConfig(`
		log_level = 1
		port = "8080"
	`)

	config, err := Read(s.T().Context(), configPath, "")
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("stateless defaults to false", func() {
		s.Falsef(config.Stateless.Get(), "Expected Stateless to default to false, got %v", config.Stateless.Get())
	})
}

func (s *ConfigSuite) TestReadConfigStatelessExplicitFalse() {
	// Test that stateless can be explicitly set to false
	configPath := s.writeConfig(`
		log_level = 1
		port = "8080"
		stateless = false
	`)

	config, err := Read(s.T().Context(), configPath, "")
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("stateless explicit false", func() {
		s.Falsef(config.Stateless.Get(), "Expected Stateless to be false, got %v", config.Stateless.Get())
	})
}

func (s *ConfigSuite) TestReadConfigValidPreservesDefaultsForMissingFields() {
	validConfigPath := s.writeConfig(`
		port = "1337"
	`)

	config, err := Read(s.T().Context(), validConfigPath, "")
	s.Require().NotNil(config)
	s.Run("reads and unmarshalls file", func() {
		s.Nil(err, "Expected nil error for valid file")
		s.Require().NotNil(config, "Expected non-nil config for valid file")
	})
	s.Run("log_level defaulted correctly", func() {
		s.Equalf(0, config.LogLevel.Get(), "Expected LogLevel to be 0, got %d", config.LogLevel.Get())
	})
	s.Run("port parsed correctly", func() {
		s.Equalf("1337", config.Port.Get(), "Expected Port to be 1337, got %s", config.Port.Get())
	})
	s.Run("list_output defaulted correctly", func() {
		s.Equalf(s.defaults.ListOutput.Get(), config.ListOutput.Get(), "Expected ListOutput to be %s, got %s", s.defaults.ListOutput.Get(), config.ListOutput.Get())
	})
	s.Run("toolsets defaulted correctly", func() {
		s.Equal(s.defaults.Toolsets.Get(), config.Toolsets.Get(), "toolsets should match defaults")
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

	sorted, err := getSortedConfigFiles(s.T().Context(), tempDir)
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

	config, err := Read(s.T().Context(), mainConfigPath, dropInDir)
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("drop-in overrides main config", func() {
		s.Equal(5, config.LogLevel.Get(), "log_level from 10-override.toml should override main")
	})

	s.Run("later drop-in overrides earlier drop-in", func() {
		s.Equal("7777", config.Port.Get(), "port from 20-final.toml should override 10-override.toml")
	})

	s.Run("preserves values not in drop-in files", func() {
		s.Equal([]string{"core", "config"}, config.Toolsets.Get(), "toolsets from main config should be preserved")
	})

	s.Run("applies all drop-in changes", func() {
		s.Equal("yaml", config.ListOutput.Get(), "list_output from 20-final.toml should be applied")
	})
}

func (s *ConfigSuite) TestDropInConfigDirAbsent() {
	mainConfigPath := s.writeConfig(`
		log_level = 3
		port = "8080"
	`)

	s.Run("missing directory is skipped", func() {
		config, err := Read(s.T().Context(), mainConfigPath, "/non/existent/directory")
		s.Require().NoError(err)
		s.Equal(3, config.LogLevel.Get())
		s.Equal("8080", config.Port.Get())
	})

	s.Run("empty directory is skipped", func() {
		config, err := Read(s.T().Context(), mainConfigPath, s.T().TempDir())
		s.Require().NoError(err)
		s.Equal(3, config.LogLevel.Get())
		s.Equal("8080", config.Port.Get())
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

	config, err := Read(s.T().Context(), mainConfigPath, dropInDir)
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("replaces arrays completely", func() {
		s.Equal([]string{"helm", "logs"}, config.Toolsets.Get(), "toolsets should be completely replaced")
		s.Equal([]string{"tool1", "tool2"}, config.EnabledTools.Get(), "enabled_tools should be preserved")
	})
}

func (s *ConfigSuite) TestSiblingConfDNotLoadedWithoutConfigDir() {
	tempDir := s.T().TempDir()

	mainConfigPath := filepath.Join(tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(mainConfigPath, []byte(`
		log_level = 1
		port = "8080"
	`), 0644))

	confDDir := filepath.Join(tempDir, "conf.d")
	s.Require().NoError(os.Mkdir(confDDir, 0755))
	s.Require().NoError(os.WriteFile(filepath.Join(confDDir, "10-override.toml"), []byte(`
		log_level = 5
		port = "9090"
	`), 0644))

	config, err := Read(s.T().Context(), mainConfigPath, "")
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("does not load sibling conf.d unless --config-dir is set", func() {
		s.Equal(1, config.LogLevel.Get())
		s.Equal("8080", config.Port.Get())
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
	config, err := Read(s.T().Context(), "", tempDir)
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("loads config from drop-in directory only", func() {
		s.Equal(5, config.LogLevel.Get(), "log_level should be from 20-override.toml")
		s.Equal("8080", config.Port.Get(), "port should be from 10-base.toml")
		s.Equal("yaml", config.ListOutput.Get(), "list_output should be from 20-override.toml")
		s.Equal([]string{"core"}, config.Toolsets.Get(), "toolsets should be from 10-base.toml")
		s.Equal(s.defaults.ReadOnly.Get(), config.ReadOnly.Get(), "unset fields keep defaults")
	})
}

func (s *ConfigSuite) TestStandaloneConfigDirAbsent() {
	s.Run("empty directory returns defaults", func() {
		config, err := Read(s.T().Context(), "", s.T().TempDir())
		s.Require().NoError(err)
		s.Equal(s.defaults.ListOutput.Get(), config.ListOutput.Get())
		s.Equal(s.defaults.Toolsets.Get(), config.Toolsets.Get())
	})

	s.Run("missing directory returns defaults", func() {
		config, err := Read(s.T().Context(), "", "/non/existent/directory")
		s.Require().NoError(err)
		s.Equal(s.defaults.ListOutput.Get(), config.ListOutput.Get())
	})

	s.Run("both paths empty returns defaults", func() {
		config, err := Read(s.T().Context(), "", "")
		s.Require().NoError(err)
		s.Equal(s.defaults.ListOutput.Get(), config.ListOutput.Get())
		s.Equal(s.defaults.Toolsets.Get(), config.Toolsets.Get())
		s.Equal(s.defaults.LogLevel.Get(), config.LogLevel.Get())
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

	config, err := Read(s.T().Context(), mainConfigPath, dropInDir)

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

	config, err := Read(s.T().Context(), mainConfigPath, absDropInDir)
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("loads from absolute drop-in path", func() {
		s.Equal(9, config.LogLevel.Get(), "log_level should be from absolute path drop-in")
		s.Equal("7777", config.Port.Get(), "port should be from absolute path drop-in")
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

	config, err := Read(s.T().Context(), mainConfigPath, notADir)

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

		deepMerge(dst, src, "src.toml", map[string]string{}, "")

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

		deepMerge(dst, src, "src.toml", map[string]string{}, "")

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

		deepMerge(dst, src, "src.toml", map[string]string{}, "")

		s.Equal("now-a-string", dst["key"], "map should be replaced by string")
	})

	s.Run("replaces arrays completely", func() {
		dst := map[string]interface{}{
			"array": []interface{}{"a", "b", "c"},
		}
		src := map[string]interface{}{
			"array": []interface{}{"x", "y"},
		}

		deepMerge(dst, src, "src.toml", map[string]string{}, "")

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

		deepMerge(dst, src, "src.toml", map[string]string{}, "")

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

	config, err := Read(s.T().Context(), mainConfigPath, dropInDir)
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("drop-in replaces denied_resources array", func() {
		s.Len(config.DeniedResources.Get(), 2, "denied_resources should have 2 entries from drop-in")
		s.Contains(config.DeniedResources.Get(), api.GroupVersionKind{
			Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole",
		})
		s.Contains(config.DeniedResources.Get(), api.GroupVersionKind{
			Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRoleBinding",
		})
	})

	s.Run("original denied_resources from main config are replaced", func() {
		s.NotContains(config.DeniedResources.Get(), api.GroupVersionKind{
			Group: "apps", Version: "v1", Kind: "Deployment",
		}, "original entry should be replaced by drop-in")
	})
}

func (s *ConfigSuite) TestRelativeConfigAndConfigDirAreIndependent() {
	tempDir := s.T().TempDir()
	s.T().Chdir(tempDir)

	configSubDir := filepath.Join("etc", "kmcp")
	s.Require().NoError(os.MkdirAll(configSubDir, 0755))
	s.Require().NoError(os.WriteFile(filepath.Join(configSubDir, "config.toml"), []byte(`
		log_level = 1
	`), 0644))

	s.Require().NoError(os.Mkdir("dropins", 0755))
	s.Require().NoError(os.WriteFile(filepath.Join("dropins", "10-override.toml"), []byte(`
		log_level = 7
		port = "3333"
	`), 0644))

	config, err := Read(s.T().Context(), filepath.Join("etc", "kmcp", "config.toml"), "dropins")
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("resolves both flags against the working directory", func() {
		s.Equal(7, config.LogLevel.Get())
		s.Equal("3333", config.Port.Get())
	})
}

func (s *ConfigSuite) TestEmptyConfigFile() {
	// Test that an empty main config file works correctly
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

	config, err := Read(s.T().Context(), mainConfigPath, confDDir)
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("applies drop-in on top of defaults when main config is empty", func() {
		s.Equal(5, config.LogLevel.Get(), "log_level should be from drop-in")
		s.Equal("9999", config.Port.Get(), "port should be from drop-in")
		// Defaults should still be applied for unset values
		s.Equal(s.defaults.ListOutput.Get(), config.ListOutput.Get(), "list_output should be default")
		s.Equal(s.defaults.Toolsets.Get(), config.Toolsets.Get(), "toolsets should be default")
	})
}

func (s *ConfigSuite) TestToolOverridesParsed() {
	configPath := s.writeConfig(`
		[tool_overrides.pods_list]
		description = "Custom pods list description"

		[tool_overrides.resources_get]
		description = "Custom resources get description"
	`)

	config, err := Read(s.T().Context(), configPath, "")
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("parses tool_overrides with multiple entries", func() {
		s.Require().Contains(config.ToolOverrides.Get(), "pods_list")
		s.Require().Contains(config.ToolOverrides.Get(), "resources_get")
		s.Equal("Custom pods list description", config.ToolOverrides.Get()["pods_list"].Description)
		s.Equal("Custom resources get description", config.ToolOverrides.Get()["resources_get"].Description)
	})
}

func (s *ConfigSuite) TestToolOverridesMatchDefaultsWhenNotSpecified() {
	configPath := s.writeConfig(`
		log_level = 1
	`)

	config, err := Read(s.T().Context(), configPath, "")
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("ToolOverrides matches defaults when not specified", func() {
		s.Equal(s.defaults.ToolOverrides.Get(), config.ToolOverrides.Get())
	})
}

func (s *ConfigSuite) TestToolOverridesDropInMerge() {
	tempDir := s.T().TempDir()

	mainConfigPath := filepath.Join(tempDir, "config.toml")
	s.Require().NoError(os.WriteFile(mainConfigPath, []byte(`
		[tool_overrides.pods_list]
		description = "Main pods description"

		[tool_overrides.resources_get]
		description = "Main resources description"
	`), 0644))

	confDDir := filepath.Join(tempDir, "conf.d")
	s.Require().NoError(os.Mkdir(confDDir, 0755))

	s.Require().NoError(os.WriteFile(filepath.Join(confDDir, "10-override.toml"), []byte(`
		[tool_overrides.pods_list]
		description = "Overridden pods description"

		[tool_overrides.events_list]
		description = "New events description"
	`), 0644))

	config, err := Read(s.T().Context(), mainConfigPath, confDDir)
	s.Require().NoError(err)
	s.Require().NotNil(config)

	s.Run("drop-in overrides existing tool override", func() {
		s.Equal("Overridden pods description", config.ToolOverrides.Get()["pods_list"].Description)
	})

	s.Run("drop-in adds new tool override", func() {
		s.Equal("New events description", config.ToolOverrides.Get()["events_list"].Description)
	})

	s.Run("preserves tool overrides not in drop-in", func() {
		s.Equal("Main resources description", config.ToolOverrides.Get()["resources_get"].Description)
	})
}

func (s *ConfigSuite) TestTokenExchangeParsing() {
	s.Run("nested configuration is parsed through the declarative accessor", func() {
		configPath := s.writeConfig(`
			[token_exchange]
			strategy = "rfc8693"
			audience = "kubernetes-api"
			scopes = ["scope"]
			subject_token_type = "urn:ietf:params:oauth:token-type:access_token"
			requested_token_type = "urn:ietf:params:oauth:token-type:access_token"

			[token_exchange.client_auth]
			method = "client_secret_basic"
			client_id = "mcp-server"
			client_secret = "secret"
		`)
		cfg, err := Read(s.T().Context(), configPath, "")
		s.Require().NoError(err)
		exchange := cfg.GetTokenExchangeConfig()
		s.Require().NotNil(exchange)
		s.Equal("rfc8693", exchange.Strategy.Get())
		s.Equal("kubernetes-api", exchange.Audience.Get())
		s.Equal([]string{"scope"}, exchange.Scopes.Get())
		s.Equal("urn:ietf:params:oauth:token-type:access_token", exchange.SubjectTokenType.Get())
		s.Equal("urn:ietf:params:oauth:token-type:access_token", exchange.RequestedTokenType.Get())
		s.Require().NotNil(exchange.GetClientAuth())
		s.Equal("mcp-server", exchange.GetClientAuth().ClientID.Get())
		s.Equal("secret", exchange.GetClientAuth().ClientSecret.Get())
	})

	s.Run("absent block leaves token exchange disabled", func() {
		configPath := s.writeConfig("")
		cfg, err := Read(s.T().Context(), configPath, "")
		s.Require().NoError(err)
		s.True(cfg.GetTokenExchangeConfig() == nil)
	})

	s.Run("removed keys identify their replacement", func() {
		configPath := s.writeConfig(`sts_client_id = "mcp-server"`)
		_, err := Read(s.T().Context(), configPath, "")
		s.Require().Error(err)
		s.Contains(err.Error(), "removed config key")
		s.Contains(err.Error(), "sts_client_id")
		s.Contains(err.Error(), "token_exchange.client_auth.client_id")
	})

	s.Run("unknown nested keys are rejected", func() {
		configPath := s.writeConfig(`
			[token_exchange]
			strategy = "rfc8693"

			[token_exchange.client_auth]
			methd = "client_secret_basic"
		`)
		_, err := Read(s.T().Context(), configPath, "")
		s.Require().Error(err)
		s.Contains(err.Error(), "unknown config key")
		s.Contains(err.Error(), "token_exchange.client_auth.methd")
	})

	s.Run("unknown top-level key parked in a table is rejected", func() {
		_, err := ReadToml([]byte(`
[http]
read_header_timeout = "10s"
port = "8080"
`))
		s.Require().Error(err)
		s.Contains(err.Error(), "unknown config key")
		s.Contains(err.Error(), "http.port")
	})

	s.Run("unknown field in prompts is rejected", func() {
		_, err := ReadToml([]byte(`
[[prompts]]
name = "k8s-troubleshoot"
typo = "oops"
`))
		s.Require().Error(err)
		s.Contains(err.Error(), "unknown config key")
		s.Contains(err.Error(), "typo")
	})

	s.Run("unknown field in confirmation_rules is rejected", func() {
		_, err := ReadToml([]byte(`
[[confirmation_rules]]
tool = "helm_uninstall"
message = "uninstall"
typo = "oops"
`))
		s.Require().Error(err)
		s.Contains(err.Error(), "unknown config key")
		s.Contains(err.Error(), "typo")
	})

	s.Run("unknown field in tool_overrides is rejected", func() {
		_, err := ReadToml([]byte(`
[tool_overrides.pods_list]
description = "list pods"
typo = "oops"
`))
		s.Require().Error(err)
		s.Contains(err.Error(), "unknown config key")
		s.Contains(err.Error(), "typo")
	})
}

func (s *ConfigSuite) TestEnvAndSource() {
	s.Run("env overrides TOML and records SourceEnv", func() {
		s.T().Setenv("KUBE_CLIENT_QPS", "50")
		cfg, err := ReadToml([]byte(`kube_client_qps = 10.0`))
		s.Require().NoError(err)
		s.Equal(float32(50), cfg.KubeClientQPS.Get())
		s.Equal(SourceEnv, cfg.KubeClientQPS.Source())
		s.Contains(cfg.KubeClientQPS.Describe(), "<Env>")
	})

	s.Run("empty env does not override TOML", func() {
		s.T().Setenv("KUBE_CLIENT_QPS", "")
		cfg, err := ReadToml([]byte(`kube_client_qps = 10.0`))
		s.Require().NoError(err)
		s.Equal(float32(10), cfg.KubeClientQPS.Get())
		s.NotEqual(SourceEnv, cfg.KubeClientQPS.Source())
	})

	s.Run("file source is the path", func() {
		path := s.writeConfig(`port = "9090"`)
		cfg, err := Read(s.T().Context(), path, "")
		s.Require().NoError(err)
		s.Equal("9090", cfg.Port.Get())
		s.Equal(Source(path), cfg.Port.Source())
		s.Contains(cfg.Port.Describe(), path)
	})

	s.Run("sensitive values are redacted", func() {
		cfg, err := ReadToml([]byte(`
[token_exchange.client_auth]
client_secret = "super-secret"
`))
		s.Require().NoError(err)
		s.Equal("<redacted>", cfg.TokenExchange.ClientAuth.ClientSecret.String())
		s.Contains(cfg.TokenExchange.ClientAuth.ClientSecret.Describe(), "<redacted>")
	})
}

func (s *ConfigSuite) TestPinNonReloadable() {
	prev, err := ReadToml([]byte(`port = "8080"`))
	s.Require().NoError(err)
	next, err := ReadToml([]byte(`port = "9090"`), WithPrevious(prev))
	s.Require().NoError(err)
	s.Equal("8080", next.Port.Get())
	s.Equal(prev.Port.Source(), next.Port.Source())
}

func (s *ConfigSuite) TestConfirmationRulesDefaults() {
	configPath := s.writeConfig(``)
	config, err := Read(s.T().Context(), configPath, "")
	s.Require().NoError(err)
	s.Run("default fallback is allow", func() {
		s.Equal("allow", config.ConfirmationFallback.Get())
	})
	s.Run("default rules is empty", func() {
		s.Empty(config.ConfirmationRules.Get())
	})
}

func (s *ConfigSuite) TestConfirmationRulesParsing() {
	configPath := s.writeConfig(`
		confirmation_fallback = "deny"

		[[confirmation_rules]]
		tool = "helm_uninstall"
		message = "This will uninstall a Helm release."

		[[confirmation_rules]]
		destructive = true
		message = "Destructive operation."

		[[confirmation_rules]]
		verb = "delete"
		namespace = "kube-system"
		message = "Deleting in kube-system."

		[[confirmation_rules]]
		verb = "get"
		kind = "Secret"
		message = "Accessing a Secret."
	`)
	config, err := Read(s.T().Context(), configPath, "")
	s.Require().NoError(err)
	s.Run("confirmation_fallback parsed correctly", func() {
		s.Equal("deny", config.ConfirmationFallback.Get())
	})
	s.Run("all rules parsed", func() {
		s.Len(config.ConfirmationRules.Get(), 4)
	})
	s.Run("tool-level rule parsed", func() {
		r := config.ConfirmationRules.Get()[0]
		s.Equal("helm_uninstall", r.Tool)
		s.Equal("This will uninstall a Helm release.", r.Message)
	})
	s.Run("destructive rule parsed", func() {
		r := config.ConfirmationRules.Get()[1]
		s.Require().NotNil(r.Destructive)
		s.True(*r.Destructive)
	})
	s.Run("kube-level rule parsed", func() {
		r := config.ConfirmationRules.Get()[2]
		s.Equal("delete", r.Verb)
		s.Equal("kube-system", r.Namespace)
	})
	s.Run("kube-level rule with kind parsed", func() {
		r := config.ConfirmationRules.Get()[3]
		s.Equal("get", r.Verb)
		s.Equal("Secret", r.Kind)
	})
}

func (s *ConfigSuite) TestGetTLSConfig() {
	s.Run("returns TOML value when env is unset", func() {
		s.Require().NoError(os.Unsetenv(EnvTLSMinVersion))
		s.Require().NoError(os.Unsetenv(EnvTLSCipherSuites))
		cfg := New()
		cfg.TLSMinVersion.SetForTest("1.3")
		cfg.TLSCipherSuites.SetForTest([]string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"})
		s.Equal("1.3", cfg.TLSMinVersion.Get())
		s.Equal(cfg.TLSCipherSuites.Get(), cfg.TLSCipherSuites.Get())
	})

	s.Run("env overrides TOML at load", func() {
		s.T().Setenv(EnvTLSMinVersion, "1.3")
		s.T().Setenv(EnvTLSCipherSuites, "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384")
		cfg, err := ReadToml([]byte(`
tls_min_version = "1.2"
tls_cipher_suites = ["TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"]
`))
		s.Require().NoError(err)
		s.Equal("1.3", cfg.TLSMinVersion.Get())
		s.Equal([]string{"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384"}, cfg.TLSCipherSuites.Get())
	})

	s.Run("parses comma-separated TLS_CIPHER_SUITES env var", func() {
		s.T().Setenv(EnvTLSCipherSuites, "SUITE_A,SUITE_B")
		cfg, err := ReadToml(nil)
		s.Require().NoError(err)
		s.Equal([]string{"SUITE_A", "SUITE_B"}, cfg.TLSCipherSuites.Get())
	})

	s.Run("trims whitespace from TLS_CIPHER_SUITES env var", func() {
		s.T().Setenv(EnvTLSCipherSuites, " SUITE_A , SUITE_B ")
		cfg, err := ReadToml(nil)
		s.Require().NoError(err)
		s.Equal([]string{"SUITE_A", "SUITE_B"}, cfg.TLSCipherSuites.Get())
	})
}

func TestConfig(t *testing.T) {
	suite.Run(t, new(ConfigSuite))
}
