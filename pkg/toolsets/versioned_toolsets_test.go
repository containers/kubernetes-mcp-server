package toolsets

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
	"k8s.io/utils/ptr"
)

type VersionedToolsetsSuite struct {
	suite.Suite
	originalToolsets []api.Toolset
}

func (s *VersionedToolsetsSuite) SetupTest() {
	s.originalToolsets = Toolsets()
	Clear()
}

func (s *VersionedToolsetsSuite) TearDownTest() {
	Clear()
	for _, toolset := range s.originalToolsets {
		Register(toolset)
	}
}

// MockToolset is a configurable toolset for testing version filtering
type MockToolset struct {
	name        string
	description string
	version     api.Version
	tools       []api.ServerTool
	prompts     []api.ServerPrompt
}

func (t *MockToolset) GetName() string                        { return t.name }
func (t *MockToolset) GetDescription() string                 { return t.description }
func (t *MockToolset) GetVersion() api.Version                { return t.version }
func (t *MockToolset) GetTools(_ api.Openshift) []api.ServerTool { return t.tools }
func (t *MockToolset) GetPrompts() []api.ServerPrompt         { return t.prompts }

var _ api.Toolset = (*MockToolset)(nil)

func (s *VersionedToolsetsSuite) TestVersionedToolsetFromString() {
	s.Run("returns nil for non-existent toolset", func() {
		result := VersionedToolsetFromString("non-existent", api.VersionGA)
		s.Nil(result)
	})

	s.Run("returns versioned toolset for existing toolset without version suffix", func() {
		Register(&MockToolset{name: "core", version: api.VersionGA})
		result := VersionedToolsetFromString("core", api.VersionBeta)
		s.NotNil(result)
		s.Equal("core", result.GetName())
		s.Equal(api.VersionBeta, result.MinVersion)
	})

	s.Run("parses toolset:version format correctly", func() {
		Register(&MockToolset{name: "helm", version: api.VersionGA})

		s.Run("alpha version", func() {
			result := VersionedToolsetFromString("helm:alpha", api.VersionGA)
			s.NotNil(result)
			s.Equal("helm", result.GetName())
			s.Equal(api.VersionAlpha, result.MinVersion)
		})

		s.Run("beta version", func() {
			result := VersionedToolsetFromString("helm:beta", api.VersionGA)
			s.NotNil(result)
			s.Equal(api.VersionBeta, result.MinVersion)
		})

		s.Run("ga version", func() {
			result := VersionedToolsetFromString("helm:ga", api.VersionAlpha)
			s.NotNil(result)
			s.Equal(api.VersionGA, result.MinVersion)
		})

		s.Run("stable version (alias for ga)", func() {
			result := VersionedToolsetFromString("helm:stable", api.VersionAlpha)
			s.NotNil(result)
			s.Equal(api.VersionGA, result.MinVersion)
		})
	})

	s.Run("falls back to default version for invalid version string", func() {
		Register(&MockToolset{name: "metrics", version: api.VersionGA})
		result := VersionedToolsetFromString("metrics:invalid", api.VersionBeta)
		s.NotNil(result)
		s.Equal("metrics", result.GetName())
		s.Equal(api.VersionBeta, result.MinVersion)
	})

	s.Run("trims whitespace from input", func() {
		Register(&MockToolset{name: "config", version: api.VersionGA})
		result := VersionedToolsetFromString("  config:alpha  ", api.VersionGA)
		s.NotNil(result)
		s.Equal("config", result.GetName())
		s.Equal(api.VersionAlpha, result.MinVersion)
	})

	s.Run("handles case-insensitive version strings", func() {
		Register(&MockToolset{name: "test", version: api.VersionGA})

		result := VersionedToolsetFromString("test:ALPHA", api.VersionGA)
		s.NotNil(result)
		s.Equal(api.VersionAlpha, result.MinVersion)

		result = VersionedToolsetFromString("test:Beta", api.VersionGA)
		s.NotNil(result)
		s.Equal(api.VersionBeta, result.MinVersion)
	})
}

func (s *VersionedToolsetsSuite) TestGetTools() {
	alphaTool := api.ServerTool{
		Tool:    api.Tool{Name: "alpha-tool"},
		Version: ptr.To(api.VersionAlpha),
	}
	betaTool := api.ServerTool{
		Tool:    api.Tool{Name: "beta-tool"},
		Version: ptr.To(api.VersionBeta),
	}
	gaTool := api.ServerTool{
		Tool:    api.Tool{Name: "ga-tool"},
		Version: ptr.To(api.VersionGA),
	}
	noVersionTool := api.ServerTool{
		Tool: api.Tool{Name: "no-version-tool"},
		// Version is nil - should inherit from toolset
	}

	s.Run("filters tools by minimum version", func() {
		toolset := &MockToolset{
			name:    "test",
			version: api.VersionGA,
			tools:   []api.ServerTool{alphaTool, betaTool, gaTool},
		}
		Register(toolset)

		s.Run("MinVersion=alpha includes all tools", func() {
			vt := VersionedToolsetFromString("test:alpha", api.VersionGA)
			tools := vt.GetTools(nil)
			s.Len(tools, 3)
		})

		s.Run("MinVersion=beta excludes alpha tools", func() {
			vt := VersionedToolsetFromString("test:beta", api.VersionGA)
			tools := vt.GetTools(nil)
			s.Len(tools, 2)
			for _, tool := range tools {
				s.NotEqual("alpha-tool", tool.Tool.Name)
			}
		})

		s.Run("MinVersion=ga excludes alpha and beta tools", func() {
			vt := VersionedToolsetFromString("test:ga", api.VersionGA)
			tools := vt.GetTools(nil)
			s.Len(tools, 1)
			s.Equal("ga-tool", tools[0].Tool.Name)
		})
	})

	s.Run("tool at exactly MinVersion is included", func() {
		toolset := &MockToolset{
			name:    "boundary",
			version: api.VersionGA,
			tools:   []api.ServerTool{betaTool},
		}
		Register(toolset)

		vt := VersionedToolsetFromString("boundary:beta", api.VersionGA)
		tools := vt.GetTools(nil)
		s.Len(tools, 1)
		s.Equal("beta-tool", tools[0].Tool.Name)
	})

	s.Run("tool without explicit version inherits toolset version", func() {
		s.Run("toolset is GA, tool included when MinVersion=ga", func() {
			toolset := &MockToolset{
				name:    "inherit-ga",
				version: api.VersionGA,
				tools:   []api.ServerTool{noVersionTool},
			}
			Register(toolset)

			vt := VersionedToolsetFromString("inherit-ga:ga", api.VersionGA)
			tools := vt.GetTools(nil)
			s.Len(tools, 1)
		})

		s.Run("toolset is beta, tool excluded when MinVersion=ga", func() {
			toolset := &MockToolset{
				name:    "inherit-beta",
				version: api.VersionBeta,
				tools:   []api.ServerTool{noVersionTool},
			}
			Register(toolset)

			vt := VersionedToolsetFromString("inherit-beta:ga", api.VersionGA)
			tools := vt.GetTools(nil)
			s.Empty(tools)
		})

		s.Run("toolset is alpha, tool included when MinVersion=alpha", func() {
			toolset := &MockToolset{
				name:    "inherit-alpha",
				version: api.VersionAlpha,
				tools:   []api.ServerTool{noVersionTool},
			}
			Register(toolset)

			vt := VersionedToolsetFromString("inherit-alpha:alpha", api.VersionGA)
			tools := vt.GetTools(nil)
			s.Len(tools, 1)
		})
	})

	s.Run("tool version overrides toolset version", func() {
		// Toolset is GA but has an alpha tool
		toolset := &MockToolset{
			name:    "override",
			version: api.VersionGA,
			tools:   []api.ServerTool{alphaTool, gaTool},
		}
		Register(toolset)

		// With MinVersion=beta, alpha tool should be excluded even though toolset is GA
		vt := VersionedToolsetFromString("override:beta", api.VersionGA)
		tools := vt.GetTools(nil)
		s.Len(tools, 1)
		s.Equal("ga-tool", tools[0].Tool.Name)
	})

	s.Run("returns empty slice when no tools match", func() {
		toolset := &MockToolset{
			name:    "no-match",
			version: api.VersionAlpha,
			tools:   []api.ServerTool{alphaTool},
		}
		Register(toolset)

		vt := VersionedToolsetFromString("no-match:ga", api.VersionGA)
		tools := vt.GetTools(nil)
		s.Empty(tools)
		s.NotNil(tools) // Should be empty slice, not nil
	})

	s.Run("returns empty slice when toolset has no tools", func() {
		toolset := &MockToolset{
			name:    "empty",
			version: api.VersionGA,
			tools:   []api.ServerTool{},
		}
		Register(toolset)

		vt := VersionedToolsetFromString("empty:alpha", api.VersionGA)
		tools := vt.GetTools(nil)
		s.Empty(tools)
	})

	s.Run("handles nil tools slice", func() {
		toolset := &MockToolset{
			name:    "nil-tools",
			version: api.VersionGA,
			tools:   nil,
		}
		Register(toolset)

		vt := VersionedToolsetFromString("nil-tools:alpha", api.VersionGA)
		tools := vt.GetTools(nil)
		s.Empty(tools)
	})
}

func (s *VersionedToolsetsSuite) TestGetPrompts() {
	alphaPrompt := api.ServerPrompt{
		Prompt:  api.Prompt{Name: "alpha-prompt"},
		Version: ptr.To(api.VersionAlpha),
	}
	betaPrompt := api.ServerPrompt{
		Prompt:  api.Prompt{Name: "beta-prompt"},
		Version: ptr.To(api.VersionBeta),
	}
	gaPrompt := api.ServerPrompt{
		Prompt:  api.Prompt{Name: "ga-prompt"},
		Version: ptr.To(api.VersionGA),
	}
	noVersionPrompt := api.ServerPrompt{
		Prompt: api.Prompt{Name: "no-version-prompt"},
		// Version is nil - should inherit from toolset
	}

	s.Run("filters prompts by minimum version", func() {
		toolset := &MockToolset{
			name:    "prompt-test",
			version: api.VersionGA,
			prompts: []api.ServerPrompt{alphaPrompt, betaPrompt, gaPrompt},
		}
		Register(toolset)

		s.Run("MinVersion=alpha includes all prompts", func() {
			vt := VersionedToolsetFromString("prompt-test:alpha", api.VersionGA)
			prompts := vt.GetPrompts()
			s.Len(prompts, 3)
		})

		s.Run("MinVersion=beta excludes alpha prompts", func() {
			vt := VersionedToolsetFromString("prompt-test:beta", api.VersionGA)
			prompts := vt.GetPrompts()
			s.Len(prompts, 2)
			for _, prompt := range prompts {
				s.NotEqual("alpha-prompt", prompt.Prompt.Name)
			}
		})

		s.Run("MinVersion=ga excludes alpha and beta prompts", func() {
			vt := VersionedToolsetFromString("prompt-test:ga", api.VersionGA)
			prompts := vt.GetPrompts()
			s.Len(prompts, 1)
			s.Equal("ga-prompt", prompts[0].Prompt.Name)
		})
	})

	s.Run("prompt at exactly MinVersion is included", func() {
		toolset := &MockToolset{
			name:    "prompt-boundary",
			version: api.VersionGA,
			prompts: []api.ServerPrompt{betaPrompt},
		}
		Register(toolset)

		vt := VersionedToolsetFromString("prompt-boundary:beta", api.VersionGA)
		prompts := vt.GetPrompts()
		s.Len(prompts, 1)
		s.Equal("beta-prompt", prompts[0].Prompt.Name)
	})

	s.Run("prompt without explicit version inherits toolset version", func() {
		s.Run("toolset is GA, prompt included when MinVersion=ga", func() {
			toolset := &MockToolset{
				name:    "prompt-inherit-ga",
				version: api.VersionGA,
				prompts: []api.ServerPrompt{noVersionPrompt},
			}
			Register(toolset)

			vt := VersionedToolsetFromString("prompt-inherit-ga:ga", api.VersionGA)
			prompts := vt.GetPrompts()
			s.Len(prompts, 1)
		})

		s.Run("toolset is beta, prompt excluded when MinVersion=ga", func() {
			toolset := &MockToolset{
				name:    "prompt-inherit-beta",
				version: api.VersionBeta,
				prompts: []api.ServerPrompt{noVersionPrompt},
			}
			Register(toolset)

			vt := VersionedToolsetFromString("prompt-inherit-beta:ga", api.VersionGA)
			prompts := vt.GetPrompts()
			s.Empty(prompts)
		})
	})

	s.Run("returns empty slice when toolset has no prompts", func() {
		toolset := &MockToolset{
			name:    "no-prompts",
			version: api.VersionGA,
			prompts: []api.ServerPrompt{},
		}
		Register(toolset)

		vt := VersionedToolsetFromString("no-prompts:alpha", api.VersionGA)
		prompts := vt.GetPrompts()
		s.Empty(prompts)
	})

	s.Run("handles nil prompts slice", func() {
		toolset := &MockToolset{
			name:    "nil-prompts",
			version: api.VersionGA,
			prompts: nil,
		}
		Register(toolset)

		vt := VersionedToolsetFromString("nil-prompts:alpha", api.VersionGA)
		prompts := vt.GetPrompts()
		s.Empty(prompts)
	})
}

func (s *VersionedToolsetsSuite) TestDelegatedMethods() {
	toolset := &MockToolset{
		name:        "delegated",
		description: "A test toolset for delegation",
		version:     api.VersionBeta,
	}
	Register(toolset)

	vt := VersionedToolsetFromString("delegated:alpha", api.VersionGA)

	s.Run("GetName delegates to wrapped toolset", func() {
		s.Equal("delegated", vt.GetName())
	})

	s.Run("GetDescription delegates to wrapped toolset", func() {
		s.Equal("A test toolset for delegation", vt.GetDescription())
	})

	s.Run("GetVersion delegates to wrapped toolset", func() {
		s.Equal(api.VersionBeta, vt.GetVersion())
	})
}

func (s *VersionedToolsetsSuite) TestValidateWithVersionedToolsets() {
	Register(&MockToolset{name: "core", version: api.VersionGA})
	Register(&MockToolset{name: "helm", version: api.VersionBeta})

	s.Run("validates plain toolset names", func() {
		err := Validate([]string{"core", "helm"})
		s.NoError(err)
	})

	s.Run("validates versioned toolset names", func() {
		err := Validate([]string{"core:alpha", "helm:beta"})
		s.NoError(err)
	})

	s.Run("validates mixed plain and versioned toolset names", func() {
		err := Validate([]string{"core", "helm:alpha"})
		s.NoError(err)
	})

	s.Run("rejects invalid toolset names with version suffix", func() {
		err := Validate([]string{"nonexistent:alpha"})
		s.Error(err)
		s.Contains(err.Error(), "invalid toolset name")
	})
}

func TestVersionedToolsets(t *testing.T) {
	suite.Run(t, new(VersionedToolsetsSuite))
}
