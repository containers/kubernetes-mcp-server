package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	internalhttp "github.com/containers/kubernetes-mcp-server/pkg/http"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/logging"
	"github.com/containers/kubernetes-mcp-server/pkg/mcp"
	internaloauth "github.com/containers/kubernetes-mcp-server/pkg/oauth"
	"github.com/containers/kubernetes-mcp-server/pkg/telemetry"
	"github.com/containers/kubernetes-mcp-server/pkg/tokenexchange"
	"github.com/containers/kubernetes-mcp-server/pkg/version"
)

var (
	long     = templates.LongDesc(i18n.T("Kubernetes Model Context Protocol (MCP) server"))
	examples = templates.Examples(i18n.T(`
# show this help
kubernetes-mcp-server -h

# shows version information
kubernetes-mcp-server --version

# start STDIO server
kubernetes-mcp-server

# start a Streamable HTTP server using a TOML config file
kubernetes-mcp-server --config /path/to/config.toml

# start using only a drop-in directory
kubernetes-mcp-server --config-dir /path/to/conf.d
`))
)

const (
	flagVersion   = "version"
	flagConfig    = "config"
	flagConfigDir = "config-dir"
)

type MCPServerOptions struct {
	Version    bool
	ConfigPath string
	ConfigDir  string
	Config     *config.Config

	logSink *logging.Sink
	genericiooptions.IOStreams
}

func NewMCPServerOptions(streams genericiooptions.IOStreams) *MCPServerOptions {
	return &MCPServerOptions{
		IOStreams: streams,
		Config:    config.New(),
	}
}

func NewMCPServer(streams genericiooptions.IOStreams) *cobra.Command {
	o := NewMCPServerOptions(streams)
	cmd := &cobra.Command{
		Use:     "kubernetes-mcp-server",
		Short:   "Kubernetes Model Context Protocol (MCP) server",
		Long:    long,
		Example: examples,
		RunE: func(c *cobra.Command, args []string) error {
			ctx := c.Context()
			if err := o.Complete(ctx, c); err != nil {
				return err
			}
			defer func() {
				if o.logSink != nil {
					if err := o.logSink.Close(); err != nil {
						klogutil.FromContext(ctx).Error(err, "failed to close log sink")
					}
				}
			}()
			if err := o.Validate(ctx); err != nil {
				return err
			}
			if err := o.Run(ctx); err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&o.Version, flagVersion, o.Version, "Print version information and quit")
	cmd.Flags().StringVar(&o.ConfigPath, flagConfig, o.ConfigPath, "Path of the config file.")
	cmd.Flags().StringVar(&o.ConfigDir, flagConfigDir, o.ConfigDir, "Directory of lexical .toml files. Usable alone or with --config. Omitted means no drop-ins. Relative paths are resolved against the working directory.")

	return cmd
}

func (m *MCPServerOptions) Complete(ctx context.Context, _ *cobra.Command) error {
	if cp := os.Getenv(config.ConfigPathEnvName); m.ConfigPath == "" && cp != "" {
		m.ConfigPath = cp
	}

	var err error
	if m.ConfigPath != "" || m.ConfigDir != "" {
		m.Config, err = config.Read(ctx, m.ConfigPath, m.ConfigDir)
		if err != nil {
			if m.ConfigPath != "" {
				return fmt.Errorf("failed to read config %s: %w", m.ConfigPath, err)
			}
			return fmt.Errorf("failed to read config: %w", err)
		}
	} else {
		m.Config, err = config.ReadToml(nil)
		if err != nil {
			return fmt.Errorf("failed to read config: %w", err)
		}
	}

	otelLogProvider, otelLogErr := telemetry.NewLogProvider(
		ctx, &m.Config.Telemetry, version.BinaryName, version.Version,
	)

	var sinkOpts []logging.Option
	if otelLogProvider != nil {
		otelSink := telemetry.NewLogSink(version.BinaryName, version.Version, otelLogProvider)
		sinkOpts = append(sinkOpts, logging.WithOtelLogSink(otelSink, otelLogProvider))
	}

	sink, err := logging.New(m.Config, m.Out, m.ErrOut, sinkOpts...)
	if err != nil {
		return err
	}
	m.logSink = sink

	if otelLogErr != nil {
		klogutil.FromContext(ctx).Error(otelLogErr, "Failed to create OTel log provider, log export disabled")
	}

	return nil
}

func (m *MCPServerOptions) Validate(ctx context.Context) error {
	return m.validateConfig(ctx, m.Config)
}

func (m *MCPServerOptions) validateConfig(ctx context.Context, cfg *config.Config) error {
	return cfg.
		WithProviderStrategies(kubernetes.GetRegisteredStrategies()).
		WithTokenExchangeStrategies(tokenexchange.GetRegisteredStrategies()).
		Validate(ctx)
}

func (m *MCPServerOptions) Run(ctx context.Context) error {
	cleanup, _ := telemetry.InitTracerWithConfig(ctx, &m.Config.Telemetry, version.BinaryName, version.Version)
	defer cleanup()

	strategy := m.Config.ClusterProviderStrategy.Get()
	if strategy == "" {
		if m.Config.KubeConfig.Get() != "" {
			strategy = "auto-detect"
		} else {
			strategy = "auto-detect (it is recommended to set this explicitly in your Config)"
		}
	}

	klogutil.FromContext(ctx).V(1).Info("Starting kubernetes-mcp-server",
		"config.path", m.ConfigPath,
		"config.toolsets", m.Config.Toolsets.Get(),
		"config.list_output", m.Config.ListOutput.Get(),
		"config.read_only", m.Config.ReadOnly.Get(),
		"config.disable_destructive", m.Config.DisableDestructive.Get(),
		"config.stateless", m.Config.Stateless.Get(),
		"config.telemetry.enabled", m.Config.Telemetry.IsEnabled(),
		"config.cluster_provider_strategy", strategy,
	)

	if m.Version {
		_, _ = fmt.Fprintf(m.Out, "%s\n", version.Version)
		return nil
	}

	oidcProvider, httpClient, err := internaloauth.CreateOIDCProviderAndClient(m.Config)
	if err != nil {
		return err
	}
	oauthState := internaloauth.NewState(internaloauth.SnapshotFromConfig(m.Config, oidcProvider, httpClient))
	cfgState := config.NewConfigState(m.Config)

	provider, err := kubernetes.NewProvider(
		ctx,
		m.Config,
		kubernetes.WithTokenExchange(oauthState),
		kubernetes.WithConfigProvider(func() *config.Config {
			return cfgState.Load()
		}),
	)
	if err != nil {
		return fmt.Errorf("unable to create kubernetes target provider: %w", err)
	}

	mcpServer, err := mcp.NewServer(ctx, mcp.Configuration{
		Config:    m.Config,
		SDKLogger: m.logSink.SDKLogger(),
	}, provider)
	if err != nil {
		return fmt.Errorf("failed to initialize MCP server: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := mcpServer.Shutdown(shutdownCtx); err != nil {
			klogutil.FromContext(ctx).Error(err, "MCP server shutdown error")
		}
	}()

	if m.ConfigPath != "" || m.ConfigDir != "" {
		stopSIGHUP := m.setupSIGHUPHandler(ctx, mcpServer, oauthState, cfgState)
		defer stopSIGHUP()
	}

	if m.Config.Port.Get() != "" {
		return internalhttp.Serve(ctx, mcpServer, cfgState, oauthState)
	}

	if err := mcpServer.ServeStdio(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}

func (m *MCPServerOptions) setupSIGHUPHandler(
	ctx context.Context,
	mcpServer *mcp.Server,
	oauthState *internaloauth.State,
	cfgState *config.ConfigState,
) (stop func()) {
	sigHupCh := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(sigHupCh, syscall.SIGHUP)

	logger := klogutil.FromContext(ctx)

	go func() {
		defer close(done)
		for range sigHupCh {
			logger.V(1).Info("Received SIGHUP signal, reloading configuration...")

			newConfig, err := config.Read(ctx, m.ConfigPath, m.ConfigDir,
				config.WithPrevious(cfgState.Load()),
			)
			if err != nil {
				logger.Error(err, "Failed to reload configuration from disk")
				continue
			}

			if err := mcpServer.ReloadConfiguration(ctx, newConfig); err != nil {
				logger.Error(err, "Failed to apply reloaded configuration")
				continue
			}

			if m.logSink != nil {
				if err := m.logSink.Reload(newConfig); err != nil {
					logger.Error(err, "Failed to reload log destination, keeping previous one")
				}
			}
			cfgState.Store(newConfig)

			currentSnapshot := oauthState.Load()
			if currentSnapshot == nil {
				currentSnapshot = &internaloauth.Snapshot{}
			}
			newSnapshot := internaloauth.SnapshotFromConfig(newConfig, currentSnapshot.OIDCProvider, currentSnapshot.HTTPClient)
			if currentSnapshot.HasProviderConfigChanged(newSnapshot) {
				logger.V(1).Info("OAuth configuration changed, recreating OIDC provider...")
				newProvider, newClient, err := internaloauth.CreateOIDCProviderAndClient(newConfig)
				if err != nil {
					logger.Error(err, "Failed to recreate OIDC provider during reload")
					continue
				}
				newSnapshot.OIDCProvider = newProvider
				newSnapshot.HTTPClient = newClient
				oauthState.Store(newSnapshot)
				logger.V(1).Info("OIDC provider and HTTP client updated successfully")
			} else if currentSnapshot.HasWellKnownConfigChanged(newSnapshot) {
				oauthState.Store(newSnapshot)
				logger.V(1).Info("OAuth well-known configuration updated")
			}

			logger.V(1).Info("Configuration reloaded successfully via SIGHUP")
		}
	}()

	logger.V(2).Info("SIGHUP handler registered for configuration reload")

	return func() {
		signal.Stop(sigHupCh)
		close(sigHupCh)
		<-done
	}
}
