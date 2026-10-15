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
	"k8s.io/klog/v2"
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
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
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
	// exit is os.Exit in production. Tests replace it so a SIGHUP Validate
	// failure cannot kill the test process.
	exit func(int)
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
			// Close the log sink whatever happens next: Validate may fail, Run
			// may panic, the version short-circuit may exit early. The sink is
			// the only thing that holds an open fd between Complete and now.
			// Close also flushes the OTel log provider when configured.
			defer func() {
				if o.logSink != nil {
					if err := o.logSink.Close(); err != nil {
						klogutil.FromContext(ctx).Error(err, "failed to close log sink")
					}
				}
			}()
			err := o.Validate(ctx)
			o.Config.Dump(ctx, nil)
			if err != nil {
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
	// If ConfigPath was not provided on the CLI, allow an env var to specify it.
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
		m.Config, err = config.ReadToml(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to read config: %w", err)
		}
	}

	// Initialize the OTel log provider before wiring klog. This runs before
	// klog is configured, so it does not use klog internally. If it fails or
	// telemetry is disabled, otelLogProvider is nil and logging proceeds
	// text-only.
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

	// klog is now wired — log the deferred OTel provider error if one occurred.
	if otelLogErr != nil {
		klogutil.FromContext(ctx).Error(otelLogErr, "Failed to create OTel log provider, log export disabled")
	}

	return nil
}

func (m *MCPServerOptions) Validate(ctx context.Context) error {
	return m.validateConfig(ctx, m.Config)
}

func (m *MCPServerOptions) validateConfig(ctx context.Context, cfg *config.Config) error {
	return errors.Join(
		toolsets.Validate(cfg.Toolsets.Get()),
		cfg.
			WithProviderStrategies(kubernetes.GetRegisteredStrategies()).
			WithTokenExchangeStrategies(tokenexchange.GetRegisteredStrategies()).
			Validate(ctx),
	)
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
		"config.disable_localhost_protection", m.Config.DisableLocalhostProtection.Get(),
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

	// Set up SIGHUP handler for configuration reload. The returned stop
	// function unregisters the signal handler and waits for the goroutine
	// to drain — important because the goroutine accesses m.logSink, which
	// the deferred Close in NewMCPServer's RunE would otherwise race with.
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

// setupSIGHUPHandler sets up a signal handler to reload configuration on SIGHUP.
// Returns a stop function that should be called to clean up the handler.
// The stop function waits for the handler goroutine to finish.
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

			// Reload config from files
			newConfig, err := config.Read(ctx, m.ConfigPath, m.ConfigDir,
				config.WithPrevious(cfgState.Load()),
			)
			if err != nil {
				logger.Error(err, "Failed to reload configuration")
				m.exitProcess(1)
				continue
			}

			prev := cfgState.Load()
			if err := m.validateConfig(ctx, newConfig); err != nil {
				logger.Error(err, "Failed to apply reloaded configuration")
				newConfig.Dump(ctx, prev)
				m.exitProcess(1)
				continue
			}

			prevOAuth := oauthState.Load()
			if prevOAuth == nil {
				prevOAuth = &internaloauth.Snapshot{}
			}

			// Discover / rebuild the OAuth snapshot before publishing config so
			// HTTP auth never observes new flags with the old provider.
			if prevOAuth.HasProviderConfigChanged(internaloauth.SnapshotFromConfig(newConfig, prevOAuth.OIDCProvider, prevOAuth.HTTPClient)) {
				logger.V(1).Info("OAuth configuration changed, recreating OIDC provider...")
			}
			nextOAuth, err := internaloauth.SnapshotForReload(prevOAuth, newConfig)
			if err != nil {
				logger.Error(err, "Failed to recreate OIDC provider during reload")
				newConfig.Dump(ctx, prev)
				continue
			}
			oauthChanged := prevOAuth.HasWellKnownConfigChanged(nextOAuth)

			if oauthChanged {
				oauthState.Store(nextOAuth)
			}
			cfgState.Store(newConfig)

			err = mcpServer.ReloadConfiguration(ctx, newConfig)
			if err != nil {
				logger.Error(err, "Failed to apply reloaded configuration")
				cfgState.Store(prev)
				if oauthChanged {
					oauthState.Store(prevOAuth)
				}
			} else if m.logSink != nil {
				// Re-apply the log destination so log_file changes and file
				// rotations are handled correctly. Failures are logged but never
				// fatal — the previous destination is preserved. logSink can be
				// nil in tests that exercise the SIGHUP handler in isolation.
				if reloadErr := m.logSink.Reload(newConfig); reloadErr != nil {
					logger.Error(reloadErr, "Failed to reload log destination, keeping previous one")
				}
			}
			newConfig.Dump(ctx, prev)
			if err != nil {
				if errors.Is(err, mcp.ErrReloadRejected) {
					m.exitProcess(1)
				}
				continue
			}
			if oauthChanged {
				if prevOAuth.HasProviderConfigChanged(nextOAuth) {
					logger.V(1).Info("OIDC provider and HTTP client updated successfully")
				} else {
					logger.V(1).Info("OAuth well-known configuration updated")
				}
			}

			logger.V(1).Info("Configuration reloaded successfully via SIGHUP")
		}
	}()

	logger.V(2).Info("SIGHUP handler registered for configuration reload")

	return func() {
		signal.Stop(sigHupCh)
		close(sigHupCh)
		<-done // Wait for goroutine to finish
	}
}

func (m *MCPServerOptions) exitProcess(code int) {
	klog.Flush()
	if m.exit != nil {
		m.exit(code)
		return
	}
	os.Exit(code)
}
