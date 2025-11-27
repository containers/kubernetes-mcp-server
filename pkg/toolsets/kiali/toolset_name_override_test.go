package kiali

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	"github.com/stretchr/testify/assert"
)

func TestKialiToolsetNameOverridesToOssm(t *testing.T) {
	// Swap the default overrides provider to simulate downstream override
	orig := config.DefaultOverridesProvider
	config.DefaultOverridesProvider = func() config.StaticConfig {
		return config.StaticConfig{
			ToolsetKialiName: "ossm",
		}
	}
	defer func() { config.DefaultOverridesProvider = orig }()

	// Snapshot current registry to restore after test
	originalToolsets := toolsets.Toolsets()
	defer func() {
		toolsets.Clear()
		for _, ts := range originalToolsets {
			toolsets.Register(ts)
		}
	}()

	// Ensure a clean registry and register a fresh instance of this toolset
	toolsets.Clear()
	toolsets.Register(&Toolset{})

	names := toolsets.ToolsetNames()
	assert.Contains(t, names, "ossm", "expected toolset name to be overridden to 'ossm'")
	assert.NotContains(t, names, "kiali", "expected original toolset name 'kiali' to be replaced")
}

func TestKialiToolsetNameDefaultsToKiali(t *testing.T) {
	// Force no overrides to be returned
	orig := config.DefaultOverridesProvider
	config.DefaultOverridesProvider = func() config.StaticConfig {
		return config.StaticConfig{}
	}
	defer func() { config.DefaultOverridesProvider = orig }()

	// Snapshot current registry to restore after test
	originalToolsets := toolsets.Toolsets()
	defer func() {
		toolsets.Clear()
		for _, ts := range originalToolsets {
			toolsets.Register(ts)
		}
	}()

	toolsets.Clear()
	toolsets.Register(&Toolset{})

	names := toolsets.ToolsetNames()
	assert.Contains(t, names, "kiali", "expected default toolset name to be 'kiali' when no override provided")
	assert.NotContains(t, names, "ossm", "did not expect 'ossm' without override")
}
