package toolsets

import (
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

type VersionedToolset struct {
	Toolset    api.Toolset
	MinVersion api.Version
}

func VersionedToolsetFromString(name string, defaultMinVersion api.Version) *VersionedToolset {
	parts := strings.SplitN(strings.TrimSpace(name), ":", 2)
	toolsetName := parts[0]

	for _, toolset := range Toolsets() {
		if toolset.GetName() == toolsetName {
			result := &VersionedToolset{Toolset: toolset, MinVersion: defaultMinVersion}
			if len(parts) == 2 {
				var version api.Version
				if err := version.UnmarshalText([]byte(parts[1])); err == nil {
					result.MinVersion = version
				}
			}
			return result
		}
	}

	return nil
}

var _ api.Toolset = &VersionedToolset{}

func (v *VersionedToolset) GetName() string {
	return v.Toolset.GetName()
}

func (v *VersionedToolset) GetDescription() string {
	return v.Toolset.GetDescription()
}

func (v *VersionedToolset) GetTools(o api.Openshift) []api.ServerTool {
	defaultVersion := v.Toolset.GetVersion()

	allTools := v.Toolset.GetTools(o)
	tools := make([]api.ServerTool, 0, len(allTools))

	for _, t := range allTools {
		version := defaultVersion
		if t.Version != nil {
			version = *t.Version
		}

		if version >= v.MinVersion {
			tools = append(tools, t)
		}
	}

	return tools

}

func (v *VersionedToolset) GetPrompts() []api.ServerPrompt {
	defaultVersion := v.Toolset.GetVersion()

	allPrompts := v.Toolset.GetPrompts()
	prompts := make([]api.ServerPrompt, 0, len(allPrompts))

	for _, p := range allPrompts {
		version := defaultVersion
		if p.Version != nil {
			version = *p.Version
		}

		if version >= v.MinVersion {
			prompts = append(prompts, p)
		}
	}

	return prompts
}

func (v *VersionedToolset) GetVersion() api.Version {
	return v.Toolset.GetVersion()
}
