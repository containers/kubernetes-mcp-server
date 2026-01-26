package cmd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	internalhttp "github.com/containers/kubernetes-mcp-server/pkg/http"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/mcp"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/containers/kubernetes-mcp-server/pkg/telemetry"
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

# start with explicit cluster provider strategy (kubeconfig, in-cluster, kcp, or disabled)
kubernetes-mcp-server --cluster-provider kubeconfig

# start with kcp cluster provider for multi-workspace support
kubernetes-mcp-server --cluster-provider kcp
`))
)

const (
	flagVersion              = "version"
	flagLogLevel             = "log-level"
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
	flagServerUrl            = "server-url"
	flagCertificateAuthority = "certificate-authority"
	flagDisableMultiCluster  = "disable-multi-cluster"
	flagClusterProvider      = "cluster-provider"
)

type MCPServerOptions struct {
	Version              bool
	LogLevel             int
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
	CertificateAuthority string
	ServerURL            string
	DisableMultiCluster  bool
	ClusterProvider      string

	ConfigPath   string
	ConfigDir    string
	StaticConfig *config.StaticConfig

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
	cmd.Flags().StringVar(&o.ServerURL, flagServerUrl, o.ServerURL, "Server URL of this application. Optional. If set, this url will be served in protected resource metadata endpoint and tokens will be validated with this audience. If not set, expected audience is kubernetes-mcp-server. Only valid if require-oauth is enabled.")
	_ = cmd.Flags().MarkHidden(flagServerUrl)
	cmd.Flags().StringVar(&o.CertificateAuthority, flagCertificateAuthority, o.CertificateAuthority, "Certificate authority path to verify certificates. Optional. Only valid if require-oauth is enabled.")
	_ = cmd.Flags().MarkHidden(flagCertificateAuthority)
	cmd.Flags().BoolVar(&o.DisableMultiCluster, flagDisableMultiCluster, o.DisableMultiCluster, "Disable multi cluster tools. Optional. If true, all tools will be run against the default cluster/context.")
	cmd.Flags().StringVar(&o.ClusterProvider, flagClusterProvider, o.ClusterProvider, "Cluster provider strategy to use (one of: kubeconfig, in-cluster, kcp, disabled). If not set, the server will auto-detect based on the environment.")

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

	m.initializeLogging()

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
}

func (m *MCPServerOptions) initializeLogging() {
	flagSet := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(flagSet)
	if m.StaticConfig.Port == "" {
		// disable klog output for stdio mode
		// this is needed to avoid klog writing to stderr and breaking the protocol
		_ = flagSet.Parse([]string{"-logtostderr=false", "-alsologtostderr=false", "-stderrthreshold=FATAL"})
		return
	}
	loggerOptions := []textlogger.ConfigOption{textlogger.Output(m.Out)}
	if m.StaticConfig.LogLevel >= 0 {
		loggerOptions = append(loggerOptions, textlogger.Verbosity(m.StaticConfig.LogLevel))
		_ = flagSet.Parse([]string{"--v", strconv.Itoa(m.StaticConfig.LogLevel)})
	}
	logger := textlogger.NewLogger(textlogger.NewConfig(loggerOptions...))
	klog.SetLoggerWithOptions(logger)
}

func (m *MCPServerOptions) Validate() error {
	if output.FromString(m.StaticConfig.ListOutput) == nil {
		return fmt.Errorf("invalid output name: %s, valid names are: %s", m.StaticConfig.ListOutput, strings.Join(output.Names, ", "))
	}
	if err := toolsets.Validate(m.StaticConfig.Toolsets); err != nil {
		return err
	}
	// Validate cluster provider strategy
	if m.StaticConfig.ClusterProviderStrategy != "" {
		validStrategies := []string{api.ClusterProviderKubeConfig, api.ClusterProviderInCluster, api.ClusterProviderKcp, api.ClusterProviderDisabled}
		if !slices.Contains(validStrategies, m.StaticConfig.ClusterProviderStrategy) {
			return fmt.Errorf("invalid cluster-provider: %s, valid values are: %s", m.StaticConfig.ClusterProviderStrategy, strings.Join(validStrategies, ", "))
		}
	}
	if !m.StaticConfig.RequireOAuth && (m.StaticConfig.OAuthAudience != "" || m.StaticConfig.AuthorizationURL != "" || m.StaticConfig.ServerURL != "" || m.StaticConfig.CertificateAuthority != "") {
		return fmt.Errorf("oauth-audience, authorization-url, server-url and certificate-authority are only valid if require-oauth is enabled. Missing --port may implicitly set require-oauth to false")
	}
	if m.StaticConfig.AuthorizationURL != "" {
		u, err := url.Parse(m.StaticConfig.AuthorizationURL)
		if err != nil {
			return err
		}
		if u.Scheme != "https" && u.Scheme != "http" {
			return fmt.Errorf("--authorization-url must be a valid URL")
		}
		if u.Scheme == "http" {
			klog.Warningf("authorization-url is using http://, this is not recommended production use")
		}
	}
	// Validate that certificate_authority is a valid file
	if caValue := strings.TrimSpace(m.StaticConfig.CertificateAuthority); caValue != "" {
		if _, err := os.Stat(caValue); err != nil {
			return fmt.Errorf("certificate-authority must be a valid file path: %w", err)
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

	var oidcProvider *oidc.Provider
	var httpClient *http.Client
	if m.StaticConfig.AuthorizationURL != "" {
		ctx := context.Background()
		if m.StaticConfig.CertificateAuthority != "" {
			httpClient = &http.Client{}
			caCert, err := os.ReadFile(m.StaticConfig.CertificateAuthority)
			if err != nil {
				return fmt.Errorf("failed to read CA certificate from %s: %w", m.StaticConfig.CertificateAuthority, err)
			}
			caCertPool := x509.NewCertPool()
			if !caCertPool.AppendCertsFromPEM(caCert) {
				return fmt.Errorf("failed to append CA certificate from %s to pool", m.StaticConfig.CertificateAuthority)
			}

			if caCertPool.Equal(x509.NewCertPool()) {
				caCertPool = nil
			}

			transport := &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs: caCertPool,
				},
			}
			httpClient.Transport = transport
			ctx = oidc.ClientContext(ctx, httpClient)
		}
		provider, err := oidc.NewProvider(ctx, m.StaticConfig.AuthorizationURL)
		if err != nil {
			return fmt.Errorf("unable to setup OIDC provider: %w", err)
		}
		oidcProvider = provider
	}

	provider, err := kubernetes.NewProvider(m.StaticConfig, kubernetes.WithTokenExchange(oidcProvider, httpClient))
	if err != nil {
		return fmt.Errorf("unable to create kubernetes target provider: %w", err)
	}

	mcpServer, err := mcp.NewServer(mcp.Configuration{
		StaticConfig: m.StaticConfig,
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

	// Set up SIGHUP handler for configuration reload
	if m.ConfigPath != "" || m.ConfigDir != "" {
		m.setupSIGHUPHandler(mcpServer)
	}

	if m.StaticConfig.Port != "" {
		ctx := context.Background()
		return internalhttp.Serve(ctx, mcpServer, m.StaticConfig, oidcProvider, httpClient)
	}

	ctx := context.Background()
	if err := mcpServer.ServeStdio(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}

// setupSIGHUPHandler sets up a signal handler to reload configuration on SIGHUP.
// This is a blocking call that runs in a separate goroutine.
func (m *MCPServerOptions) setupSIGHUPHandler(mcpServer *mcp.Server) {
	sigHupCh := make(chan os.Signal, 1)
	signal.Notify(sigHupCh, syscall.SIGHUP)

	go func() {
		for range sigHupCh {
			klog.V(1).Info("Received SIGHUP signal, reloading configuration...")

			// Reload config from files
			newConfig, err := config.Read(m.ConfigPath, m.ConfigDir)
			if err != nil {
				klog.Errorf("Failed to reload configuration from disk: %v", err)
				continue
			}

			// Apply the new configuration to the MCP server
			if err := mcpServer.ReloadConfiguration(newConfig); err != nil {
				klog.Errorf("Failed to apply reloaded configuration: %v", err)
				continue
			}

			klog.V(1).Info("Configuration reloaded successfully via SIGHUP")
		}
	}()

	klog.V(2).Info("SIGHUP handler registered for configuration reload")
}
