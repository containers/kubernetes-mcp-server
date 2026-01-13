package main

import (
	"os"

	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes-mcp-server/cmd"
)

func main() {
	flags := pflag.NewFlagSet("kubernetes-mcp-server", pflag.ExitOnError)
	pflag.CommandLine = flags

	root := cmd.NewMCPServer(genericiooptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
