package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	internalhttp "github.com/containers/kubernetes-mcp-server/pkg/http"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/logging"
	"github.com/containers/kubernetes-mcp-server/pkg/mcp"
	internaloauth "github.com/containers/kubernetes-mcp-server/pkg/oauth"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
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

# start a SSE server on port 8080
kubernetes-mcp-server --port 8080

# start a SSE server on port 8443 with a public HTTPS host of example.com
kubernetes-mcp-server --port 8443 --sse-base-url https://example.com:8443

# start a SSE server on port 8080 with multi-cluster tools disabled
kubernetes-mcp-server --port 8080 --disable-multi-cluster

# start with explicit cluster provider strategy
kubernetes-mcp-server --cluster-provider kubeconfig

# start with kcp cluster provider for multi-workspace support
kubernetes-mcp-server --cluster-provider kcp
`))
)

const (
	flagVersion              = "version"
	flagLogLevel             = "log-level"
	flagLogFile              = "log-file"
	flagConfig               = "config"
	flagConfigDir            = "config-dir"
	flagPort                 = "port"
	flagSSEBaseUrl           = "sse-base-url"
	flagKubeconfig           = "kubeconfig"
	flagToolsets             = "toolsets"
	flagListOutput           = "list-output"
	flagReadOnly             = "read-only"
	flagDisableDestructive   = "disable-destructive"
	flagStateless            = "stateless"
	flagRequireOAuth         = "require-oauth"
	flagOAuthAudience        = "oauth-audience"
	flagAuthorizationURL     = "authorization-url"
	flagSkipJWTVerification  = "skip-jwt-verification"
	flagServerUrl            = "server-url"
	flagCertificateAuthority = "certificate-authority"
	flagDisableMultiCluster  = "disable-multi-cluster"
	flagClusterProvider      = "cluster-provider"
	flagTLSCert              = "tls-cert"
	flagTLSKey               = "tls-key"
	flagRequireTLS           = "require-tls"
)

type MCPServerOptions struct {
	Version              bool
	LogLevel             int
	LogFile              string
	Port                 string
	SSEBaseUrl           string
	Kubeconfig           string
	Toolsets             []string
	ListOutput           string
	ReadOnly             bool
	DisableDestructive   bool
	Stateless            bool
	RequireOAuth         bool
	OAuthAudience        string
	AuthorizationURL     string
	SkipJWTVerification  bool
	CertificateAuthority string
	ServerURL            string
	DisableMultiCluster  bool
	ClusterProvider      string
	TLSCert              string
	TLSKey               string
	RequireTLS           bool

	ConfigPath   string
	ConfigDir    string
	StaticConfig *config.StaticConfig

	logSink *logging.Sink
	genericiooptions.IOStreams
}

func NewMCPServerOptions(streams genericiooptions.IOStreams) *MCPServerOptions {
	return &MCPServerOptions{
		IOStreams:    streams,
		StaticConfig: config.Default(),
	}
}

func NewMCPServer(streams genericiooptions.IOStreams) *cobra.Command {
	o := NewMCPServerOptions(streams)
	cmd := &cobra.Command{
		Use:     "kubernetes-mcp-server [command] [options]",
		Short:   "Kubernetes Model Context Protocol (MCP) server",
		Long:    long,
		Example: examples,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(c); err != nil {
				return err
			}
			// Close the log sink whatever happens next: Validate may fail, Run
			// may panic, the version short-circuit may exit early. The sink is
			// the only thing that holds an open fd between Complete and now.
			defer func() {
				if o.logSink != nil {
					if err := o.logSink.Close(); err != nil {
						klog.Errorf("failed to close log sink: %v", err)
					}
				}
			}()
			if err := o.Validate(); err != nil {
				return err
			}
			if err := o.Run(); err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&o.Version, flagVersion, o.Version, "Print version information and quit")
	cmd.Flags().IntVar(&o.LogLevel, flagLogLevel, o.LogLevel, "Set the log level (from 0 to 9)")
	cmd.Flags().StringVar(&o.LogFile, flagLogFile, o.LogFile, "Defines the server log file path. Required for logging in stdio mode; overrides stdout in HTTP mode. Set to \"stderr\" to log to the standard error stream.")
	cmd.Flags().StringVar(&o.ConfigPath, flagConfig, o.ConfigPath, "Path of the config file.")
	cmd.Flags().StringVar(&o.ConfigDir, flagConfigDir, o.ConfigDir, "Path to drop-in configuration directory (files loaded in lexical order). Defaults to "+config.DefaultDropInConfigDir+" relative to the config file if --config is set.")
	cmd.Flags().StringVar(&o.Port, flagPort, o.Port, "Start a streamable HTTP and SSE HTTP server on the specified port (e.g. 8080)")
	cmd.Flags().StringVar(&o.SSEBaseUrl, flagSSEBaseUrl, o.SSEBaseUrl, "SSE public base URL to use when sending the endpoint message (e.g. https://example.com)")
	cmd.Flags().StringVar(&o.Kubeconfig, flagKubeconfig, o.Kubeconfig, "Path to the kubeconfig file to use for authentication")
	cmd.Flags().StringSliceVar(&o.Toolsets, flagToolsets, o.Toolsets, "Comma-separated list of MCP toolsets to use (available toolsets: "+strings.Join(toolsets.ToolsetNames(), ", ")+"). Defaults to "+strings.Join(o.StaticConfig.Toolsets, ", ")+".")
	cmd.Flags().StringVar(&o.ListOutput, flagListOutput, o.ListOutput, "Output format for resource list operations (one of: "+strings.Join(output.Names, ", ")+"). Defaults to "+o.StaticConfig.ListOutput+".")
	cmd.Flags().BoolVar(&o.ReadOnly, flagReadOnly, o.ReadOnly, "If true, only tools annotated with readOnlyHint=true are exposed")
	cmd.Flags().BoolVar(&o.DisableDestructive, flagDisableDestructive, o.DisableDestructive, "If true, tools annotated with destructiveHint=true are disabled")
	cmd.Flags().BoolVar(&o.Stateless, flagStateless, o.Stateless, "If true, run the MCP server in stateless mode (disables tool/prompt change notifications). Useful for container deployments and load balancing. Default is false (stateful mode)")
	cmd.Flags().BoolVar(&o.RequireOAuth, flagRequireOAuth, o.RequireOAuth, "If true, requires OAuth authorization as defined in the Model Context Protocol (MCP) specification. This flag is ignored if transport type is stdio")
	_ = cmd.Flags().MarkHidden(flagRequireOAuth)
	cmd.Flags().StringVar(&o.OAuthAudience, flagOAuthAudience, o.OAuthAudience, "OAuth audience for token claims validation. Optional. If not set, the audience is not validated. Only valid if require-oauth is enabled.")
	_ = cmd.Flags().MarkHidden(flagOAuthAudience)
	cmd.Flags().StringVar(&o.AuthorizationURL, flagAuthorizationURL, o.AuthorizationURL, "OAuth authorization server URL for protected resource endpoint. If not provided, the Kubernetes API server host will be used. Only valid if require-oauth is enabled.")
	_ = cmd.Flags().MarkHidden(flagAuthorizationURL)
	cmd.Flags().BoolVar(&o.SkipJWTVerification, flagSkipJWTVerification, o.SkipJWTVerification, "Skip JWT cryptographic signature verification when require-oauth is enabled but no authorization-url is configured. Only use behind a trusted reverse proxy that verifies tokens.")
	_ = cmd.Flags().MarkHidden(flagSkipJWTVerification)
	cmd.Flags().StringVar(&o.ServerURL, flagServerUrl, o.ServerURL, "Server URL of this application. Optional. If set, this url will be served in protected resource metadata endpoint and tokens will be validated with this audience. If not set, expected audience is kubernetes-mcp-server. Only valid if require-oauth is enabled.")
	_ = cmd.Flags().MarkHidden(flagServerUrl)
	cmd.Flags().StringVar(&o.CertificateAuthority, flagCertificateAuthority, o.CertificateAuthority, "Certificate authority path to verify certificates. Optional. Only valid if require-oauth is enabled.")
	_ = cmd.Flags().MarkHidden(flagCertificateAuthority)
	cmd.Flags().BoolVar(&o.DisableMultiCluster, flagDisableMultiCluster, o.DisableMultiCluster, "Disable multi cluster tools. Optional. If true, all tools will be run against the default cluster/context.")
	cmd.Flags().StringVar(&o.ClusterProvider, flagClusterProvider, o.ClusterProvider, "Cluster provider strategy to use (one of: "+strings.Join(kubernetes.GetRegisteredStrategies(), ", ")+"). If not set, the server will auto-detect based on the environment.")
	cmd.Flags().StringVar(&o.TLSCert, flagTLSCert, o.TLSCert, "Path to TLS certificate file for HTTPS. Must be used together with --tls-key.")
	cmd.Flags().StringVar(&o.TLSKey, flagTLSKey, o.TLSKey, "Path to TLS private key file for HTTPS. Must be used together with --tls-cert.")
	cmd.Flags().BoolVar(&o.RequireTLS, flagRequireTLS, o.RequireTLS, "Require TLS for server and all outbound connections")

	return cmd
}

func (m *MCPServerOptions) Complete(cmd *cobra.Command) error {
	if m.ConfigPath != "" || m.ConfigDir != "" {
		cnf, err := config.Read(m.ConfigPath, m.ConfigDir)
		if err != nil {
			return err
		}
		m.StaticConfig = cnf
	}

	m.loadFlags(cmd)

	sink, err := logging.New(m.StaticConfig, m.Out, m.ErrOut)
	if err != nil {
		return err
	}
	m.logSink = sink

	if m.StaticConfig.RequireOAuth && m.StaticConfig.Port == "" {
		// RequireOAuth is not relevant flow for STDIO transport
		m.StaticConfig.RequireOAuth = false
	}

	return nil
}

func (m *MCPServerOptions) loadFlags(cmd *cobra.Command) {
	if cmd.Flag(flagLogLevel).Changed {
		m.StaticConfig.LogLevel = m.LogLevel
	}
	if cmd.Flag(flagLogFile).Changed {
		m.StaticConfig.LogFile = m.LogFile
	}
	if cmd.Flag(flagPort).Changed {
		m.StaticConfig.Port = m.Port
	}
	if cmd.Flag(flagSSEBaseUrl).Changed {
		m.StaticConfig.SSEBaseURL = m.SSEBaseUrl
	}
	if cmd.Flag(flagKubeconfig).Changed {
		m.StaticConfig.KubeConfig = m.Kubeconfig
	}
	if cmd.Flag(flagListOutput).Changed {
		m.StaticConfig.ListOutput = m.ListOutput
	}
	if cmd.Flag(flagReadOnly).Changed {
		m.StaticConfig.ReadOnly = m.ReadOnly
	}
	if cmd.Flag(flagDisableDestructive).Changed {
		m.StaticConfig.DisableDestructive = m.DisableDestructive
	}
	if cmd.Flag(flagStateless).Changed {
		m.StaticConfig.Stateless = m.Stateless
	}
	if cmd.Flag(flagToolsets).Changed {
		m.StaticConfig.Toolsets = m.Toolsets
	}
	if cmd.Flag(flagRequireOAuth).Changed {
		m.StaticConfig.RequireOAuth = m.RequireOAuth
	}
	if cmd.Flag(flagOAuthAudience).Changed {
		m.StaticConfig.OAuthAudience = m.OAuthAudience
	}
	if cmd.Flag(flagAuthorizationURL).Changed {
		m.StaticConfig.AuthorizationURL = m.AuthorizationURL
	}
	if cmd.Flag(flagSkipJWTVerification).Changed {
		m.StaticConfig.SkipJWTVerification = m.SkipJWTVerification
	}
	if cmd.Flag(flagServerUrl).Changed {
		m.StaticConfig.ServerURL = m.ServerURL
	}
	if cmd.Flag(flagCertificateAuthority).Changed {
		m.StaticConfig.CertificateAuthority = m.CertificateAuthority
	}
	if cmd.Flag(flagClusterProvider).Changed {
		m.StaticConfig.ClusterProviderStrategy = m.ClusterProvider
	}
	if cmd.Flag(flagDisableMultiCluster).Changed && m.DisableMultiCluster {
		m.StaticConfig.ClusterProviderStrategy = api.ClusterProviderDisabled
	}
	if cmd.Flag(flagTLSCert).Changed {
		m.StaticConfig.TLSCert = m.TLSCert
	}
	if cmd.Flag(flagTLSKey).Changed {
		m.StaticConfig.TLSKey = m.TLSKey
	}
	if cmd.Flag(flagRequireTLS).Changed {
		m.StaticConfig.RequireTLS = m.RequireTLS
	}
}

func (m *MCPServerOptions) Validate() error {
	// Config-level validations (shared with SIGHUP reload)
	if err := m.StaticConfig.
		WithProviderStrategies(kubernetes.GetRegisteredStrategies()).
		WithTokenExchangeStrategies(tokenexchange.GetRegisteredStrategies()).
		Validate(); err != nil {
		return err
	}
	// CLI-level validations (flag interactions that can't change on reload)
	if m.StaticConfig.TLSCert != "" && m.StaticConfig.Port == "" {
		return fmt.Errorf("--tls-cert and --tls-key require --port to be set (TLS is only supported in HTTP mode)")
	}
	if m.StaticConfig.RequireTLS && m.StaticConfig.Port != "" {
		if m.StaticConfig.TLSCert == "" || m.StaticConfig.TLSKey == "" {
			return fmt.Errorf("require_tls is enabled but TLS certificates are not configured (set tls_cert and tls_key)")
		}
	}
	return nil
}

func (m *MCPServerOptions) Run() error {
	// Initialize OpenTelemetry tracing with config (env vars take precedence)
	cleanup, _ := telemetry.InitTracerWithConfig(&m.StaticConfig.Telemetry, version.BinaryName, version.Version)
	defer cleanup()

	klog.V(1).Info("Starting kubernetes-mcp-server")
	klog.V(1).Infof(" - Config: %s", m.ConfigPath)
	klog.V(1).Infof(" - Toolsets: %s", strings.Join(m.StaticConfig.Toolsets, ", "))
	klog.V(1).Infof(" - ListOutput: %s", m.StaticConfig.ListOutput)
	klog.V(1).Infof(" - Read-only mode: %t", m.StaticConfig.ReadOnly)
	klog.V(1).Infof(" - Disable destructive tools: %t", m.StaticConfig.DisableDestructive)
	klog.V(1).Infof(" - Stateless mode: %t", m.StaticConfig.Stateless)
	klog.V(1).Infof(" - Telemetry enabled: %t", m.StaticConfig.Telemetry.IsEnabled())

	strategy := m.StaticConfig.ClusterProviderStrategy
	if strategy == "" {
		strategy = "auto-detect (it is recommended to set this explicitly in your Config)"
	}

	klog.V(1).Infof(" - ClusterProviderStrategy: %s", strategy)

	if m.Version {
		_, _ = fmt.Fprintf(m.Out, "%s\n", version.Version)
		return nil
	}

	oidcProvider, httpClient, err := internaloauth.CreateOIDCProviderAndClient(m.StaticConfig)
	if err != nil {
		return err
	}
	oauthState := internaloauth.NewState(internaloauth.SnapshotFromConfig(m.StaticConfig, oidcProvider, httpClient))

	provider, err := kubernetes.NewProvider(m.StaticConfig, kubernetes.WithTokenExchange(oauthState))
	if err != nil {
		return fmt.Errorf("unable to create kubernetes target provider: %w", err)
	}

	mcpServer, err := mcp.NewServer(mcp.Configuration{
		StaticConfig: m.StaticConfig,
		SDKLogger:    m.logSink.SDKLogger(),
	}, provider)
	if err != nil {
		return fmt.Errorf("failed to initialize MCP server: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := mcpServer.Shutdown(shutdownCtx); err != nil {
			klog.Errorf("MCP server shutdown error: %v", err)
		}
	}()

	cfgState := config.NewStaticConfigState(m.StaticConfig)

	// Set up SIGHUP handler for configuration reload. The returned stop
	// function unregisters the signal handler and waits for the goroutine
	// to drain — important because the goroutine accesses m.logSink, which
	// the deferred Close in NewMCPServer's RunE would otherwise race with.
	if m.ConfigPath != "" || m.ConfigDir != "" {
		stopSIGHUP := m.setupSIGHUPHandler(mcpServer, oauthState, cfgState)
		defer stopSIGHUP()
	}

	if m.StaticConfig.Port != "" {
		ctx := context.Background()
		return internalhttp.Serve(ctx, mcpServer, cfgState, oauthState)
	}

	ctx := context.Background()
	if err := mcpServer.ServeStdio(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}

// setupSIGHUPHandler sets up a signal handler to reload configuration on SIGHUP.
// Returns a stop function that should be called to clean up the handler.
// The stop function waits for the handler goroutine to finish.
func (m *MCPServerOptions) setupSIGHUPHandler(mcpServer *mcp.Server, oauthState *internaloauth.State, cfgState *config.StaticConfigState) (stop func()) {
	sigHupCh := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(sigHupCh, syscall.SIGHUP)

	go func() {
		defer close(done)
		for range sigHupCh {
			klog.V(1).Info("Received SIGHUP signal, reloading configuration...")

			// Reload config from files
			newConfig, err := config.Read(m.ConfigPath, m.ConfigDir)
			if err != nil {
				klog.Errorf("Failed to reload configuration from disk: %v", err)
				continue
			}

			// Apply the new configuration to the MCP server first — if this fails,
			// we skip the OAuth state and config state updates to avoid inconsistent state.
			if err := mcpServer.ReloadConfiguration(newConfig); err != nil {
				klog.Errorf("Failed to apply reloaded configuration: %v", err)
				continue
			}

			// Re-apply the log destination so log_file changes and file
			// rotations are handled correctly. Failures are logged but never
			// fatal — the previous destination is preserved. logSink can be
			// nil in tests that exercise the SIGHUP handler in isolation.
			if m.logSink != nil {
				if err := m.logSink.Reload(newConfig); err != nil {
					klog.Errorf("Failed to reload log destination, keeping previous one: %v", err)
				}
			}
			// Publish the new config so the HTTP auth middleware picks it up.
			cfgState.Store(newConfig)

			// Check if OAuth-relevant config changed and update the shared state
			currentSnapshot := oauthState.Load()
			if currentSnapshot == nil {
				currentSnapshot = &internaloauth.Snapshot{}
			}
			newSnapshot := internaloauth.SnapshotFromConfig(newConfig, currentSnapshot.OIDCProvider, currentSnapshot.HTTPClient)
			if currentSnapshot.HasProviderConfigChanged(newSnapshot) {
				klog.V(1).Info("OAuth configuration changed, recreating OIDC provider...")
				newProvider, newClient, err := internaloauth.CreateOIDCProviderAndClient(newConfig)
				if err != nil {
					klog.Errorf("Failed to recreate OIDC provider during reload: %v", err)
					continue
				}
				newSnapshot.OIDCProvider = newProvider
				newSnapshot.HTTPClient = newClient
				oauthState.Store(newSnapshot)
				klog.V(1).Info("OIDC provider and HTTP client updated successfully")
			} else if currentSnapshot.HasWellKnownConfigChanged(newSnapshot) {
				oauthState.Store(newSnapshot)
				klog.V(1).Info("OAuth well-known configuration updated")
			}

			klog.V(1).Info("Configuration reloaded successfully via SIGHUP")
		}
	}()

	klog.V(2).Info("SIGHUP handler registered for configuration reload")

	return func() {
		signal.Stop(sigHupCh)
		close(sigHupCh)
		<-done // Wait for goroutine to finish
	}
}
