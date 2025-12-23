package main

import (
	"os"

	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes-mcp-server/cmd"
	"github.com/containers/kubernetes-mcp-server/pkg/telemetry"
	"github.com/containers/kubernetes-mcp-server/pkg/version"
)

func main() {
	// Initialize OpenTelemetry tracing
	cleanup, _ := telemetry.InitTracer(version.BinaryName, version.Version)
	// Tracing is optional - errors are logged in InitTracer but don't prevent startup
	defer cleanup()

	flags := pflag.NewFlagSet("kubernetes-mcp-server", pflag.ExitOnError)
	pflag.CommandLine = flags

	root := cmd.NewMCPServer(genericiooptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
