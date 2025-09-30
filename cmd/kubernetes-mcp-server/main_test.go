package main

import (
	"os"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/confluence"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/config"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/core"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/helm"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/prometheus"
)

func TestMain(m *testing.M) {
	// The blank imports above will register the core, config, and helm toolsets.
	// We need to manually register the confluence toolset as it requires configuration.
	// For this test, we can register a disabled version.
	confluenceToolset, _ := confluence.NewToolset(nil)
	toolsets.Register(confluenceToolset)

	prometheusToolset, _ := prometheus.NewToolset(nil)
	toolsets.Register(prometheusToolset)

	os.Exit(m.Run())
}

func Example_version() {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"kubernetes-mcp-server", "--version"}
	main()
	// Output: 0.0.0
}