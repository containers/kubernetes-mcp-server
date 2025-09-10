package mcp

import (
	"slices"

	"github.com/mark3labs/mcp-go/server"
)

type Toolset interface {
	GetName() string
	GetDescription() string
	GetTools(s *Server) []server.ServerTool
}

var Toolsets = []Toolset{
	&Full{},
}

var ToolsetNames []string

func ToolsetFromString(name string) Toolset {
	for _, toolset := range Toolsets {
		if toolset.GetName() == name {
			return toolset
		}
	}
	return nil
}

type Full struct{}

func (p *Full) GetName() string {
	return "full"
}
func (p *Full) GetDescription() string {
	return "Complete toolset with all tools and extended outputs"
}
func (p *Full) GetTools(s *Server) []server.ServerTool {
	return slices.Concat(
		s.initConfiguration(),
		s.initEvents(),
		s.initNamespaces(),
		s.initPods(),
		s.initResources(),
		s.initHelm(),
	)
}

func init() {
	ToolsetNames = make([]string, 0)
	for _, toolset := range Toolsets {
		ToolsetNames = append(ToolsetNames, toolset.GetName())
	}
}
