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
	"slices"
	"strconv"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	internalhttp "github.com/containers/kubernetes-mcp-server/pkg/http"
	"github.com/containers/kubernetes-mcp-server/pkg/kiali"
	"github.com/containers/kubernetes-mcp-server/pkg/mcp"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
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
`))
)

const (
	flagVersion              = "version"
	flagLogLevel             = "log-level"
	flagConfig               = "config"
	flagSSEPort              = "sse-port"
	flagHttpPort             = "http-port"
	flagPort                 = "port"
	flagSSEBaseUrl           = "sse-base-url"
	flagKubeconfig           = "kubeconfig"
	flagToolsets             = "toolsets"
	flagListOutput           = "list-output"
	flagReadOnly             = "read-only"
	flagDisableDestructive   = "disable-destructive"
	flagRequireOAuth         = "require-oauth"
	flagOAuthAudience        = "oauth-audience"
	flagValidateToken        = "validate-token"
	flagAuthorizationURL     = "authorization-url"
	flagServerUrl            = "server-url"
	flagCertificateAuthority = "certificate-authority"
	flagDisableMultiCluster  = "disable-multi-cluster"
	flagKialiUrl             = "kiali-url"
	flagKialiInsecure        = "kiali-insecure"
)

type KialiOptions struct {
	Url      string
	Insecure bool
}

type MCPServerOptions struct {
	Version              bool
	LogLevel             int
	Port                 string
	SSEPort              int
	HttpPort             int
	SSEBaseUrl           string
	Kubeconfig           string
	Toolsets             []string
	ListOutput           string
	ReadOnly             bool
	DisableDestructive   bool
	RequireOAuth         bool
	OAuthAudience        string
	ValidateToken        bool
	AuthorizationURL     string
	CertificateAuthority string
	ServerURL            string
	DisableMultiCluster  bool
	KialiOptions         KialiOptions

	ConfigPath   string
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
	cmd.Flags().IntVar(&o.SSEPort, flagSSEPort, o.SSEPort, "Start a SSE server on the specified port")
	cmd.Flag(flagSSEPort).Deprecated = "Use --port instead"
	cmd.Flags().IntVar(&o.HttpPort, flagHttpPort, o.HttpPort, "Start a streamable HTTP server on the specified port")
	cmd.Flag(flagHttpPort).Deprecated = "Use --port instead"
	cmd.Flags().StringVar(&o.Port, flagPort, o.Port, "Start a streamable HTTP and SSE HTTP server on the specified port (e.g. 8080)")
	cmd.Flags().StringVar(&o.SSEBaseUrl, flagSSEBaseUrl, o.SSEBaseUrl, "SSE public base URL to use when sending the endpoint message (e.g. https://example.com)")
	cmd.Flags().StringVar(&o.Kubeconfig, flagKubeconfig, o.Kubeconfig, "Path to the kubeconfig file to use for authentication")
	cmd.Flags().StringSliceVar(&o.Toolsets, flagToolsets, o.Toolsets, "Comma-separated list of MCP toolsets to use (available toolsets: "+strings.Join(toolsets.ToolsetNames(), ", ")+"). Defaults to "+strings.Join(o.StaticConfig.Toolsets, ", ")+".")
	cmd.Flags().StringVar(&o.ListOutput, flagListOutput, o.ListOutput, "Output format for resource list operations (one of: "+strings.Join(output.Names, ", ")+"). Defaults to "+o.StaticConfig.ListOutput+".")
	cmd.Flags().BoolVar(&o.ReadOnly, flagReadOnly, o.ReadOnly, "If true, only tools annotated with readOnlyHint=true are exposed")
	cmd.Flags().BoolVar(&o.DisableDestructive, flagDisableDestructive, o.DisableDestructive, "If true, tools annotated with destructiveHint=true are disabled")
	cmd.Flags().BoolVar(&o.RequireOAuth, flagRequireOAuth, o.RequireOAuth, "If true, requires OAuth authorization as defined in the Model Context Protocol (MCP) specification. This flag is ignored if transport type is stdio")
	_ = cmd.Flags().MarkHidden(flagRequireOAuth)
	cmd.Flags().StringVar(&o.OAuthAudience, flagOAuthAudience, o.OAuthAudience, "OAuth audience for token claims validation. Optional. If not set, the audience is not validated. Only valid if require-oauth is enabled.")
	_ = cmd.Flags().MarkHidden(flagOAuthAudience)
	cmd.Flags().BoolVar(&o.ValidateToken, flagValidateToken, o.ValidateToken, "If true, validates the token against the Kubernetes API Server using TokenReview. Optional. If not set, the token is not validated. Only valid if require-oauth is enabled.")
	_ = cmd.Flags().MarkHidden(flagValidateToken)
	cmd.Flags().StringVar(&o.AuthorizationURL, flagAuthorizationURL, o.AuthorizationURL, "OAuth authorization server URL for protected resource endpoint. If not provided, the Kubernetes API server host will be used. Only valid if require-oauth is enabled.")
	_ = cmd.Flags().MarkHidden(flagAuthorizationURL)
	cmd.Flags().StringVar(&o.ServerURL, flagServerUrl, o.ServerURL, "Server URL of this application. Optional. If set, this url will be served in protected resource metadata endpoint and tokens will be validated with this audience. If not set, expected audience is kubernetes-mcp-server. Only valid if require-oauth is enabled.")
	_ = cmd.Flags().MarkHidden(flagServerUrl)
	cmd.Flags().StringVar(&o.CertificateAuthority, flagCertificateAuthority, o.CertificateAuthority, "Certificate authority path to verify certificates. Optional. Only valid if require-oauth is enabled.")
	_ = cmd.Flags().MarkHidden(flagCertificateAuthority)
	cmd.Flags().BoolVar(&o.DisableMultiCluster, flagDisableMultiCluster, o.DisableMultiCluster, "Disable multi cluster tools. Optional. If true, all tools will be run against the default cluster/context.")
	cmd.Flags().StringVar(&o.KialiOptions.Url, flagKialiUrl, o.KialiOptions.Url, "Kiali endpoint to use for kiali tools. Optional. If not set, the kiali tools will not be available.")
	cmd.Flags().BoolVar(&o.KialiOptions.Insecure, flagKialiInsecure, o.KialiOptions.Insecure, "If true, allows insecure TLS connections to Kiali. Optional. If true, the kiali tools will not be available.")

	return cmd
}

func (m *MCPServerOptions) Complete(cmd *cobra.Command) error {
	if m.ConfigPath != "" {
		cnf, err := config.Read(m.ConfigPath)
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
	} else if cmd.Flag(flagSSEPort).Changed {
		m.StaticConfig.Port = strconv.Itoa(m.SSEPort)
	} else if cmd.Flag(flagHttpPort).Changed {
		m.StaticConfig.Port = strconv.Itoa(m.HttpPort)
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
	if cmd.Flag(flagToolsets).Changed {
		m.StaticConfig.Toolsets = m.Toolsets
	}
	if cmd.Flag(flagRequireOAuth).Changed {
		m.StaticConfig.RequireOAuth = m.RequireOAuth
	}
	if cmd.Flag(flagOAuthAudience).Changed {
		m.StaticConfig.OAuthAudience = m.OAuthAudience
	}
	if cmd.Flag(flagValidateToken).Changed {
		m.StaticConfig.ValidateToken = m.ValidateToken
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
	if cmd.Flag(flagDisableMultiCluster).Changed && m.DisableMultiCluster {
		m.StaticConfig.ClusterProviderStrategy = config.ClusterProviderDisabled
	}
	if cmd.Flag(flagKialiUrl).Changed || cmd.Flag(flagKialiInsecure).Changed {
		m.StaticConfig.SetToolsetConfig("kiali", &kiali.Config{Url: m.KialiOptions.Url, Insecure: m.KialiOptions.Insecure})
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
	if m.Port != "" && (m.SSEPort > 0 || m.HttpPort > 0) {
		return fmt.Errorf("--port is mutually exclusive with deprecated --http-port and --sse-port flags")
	}
	if output.FromString(m.StaticConfig.ListOutput) == nil {
		return fmt.Errorf("invalid output name: %s, valid names are: %s", m.StaticConfig.ListOutput, strings.Join(output.Names, ", "))
	}
	if err := toolsets.Validate(m.StaticConfig.Toolsets); err != nil {
		return err
	}
	if !m.StaticConfig.RequireOAuth && (m.StaticConfig.ValidateToken || m.StaticConfig.OAuthAudience != "" || m.StaticConfig.AuthorizationURL != "" || m.StaticConfig.ServerURL != "" || m.StaticConfig.CertificateAuthority != "") {
		return fmt.Errorf("validate-token, oauth-audience, authorization-url, server-url and certificate-authority are only valid if require-oauth is enabled. Missing --port may implicitly set require-oauth to false")
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
    /* If Kiali tools are enabled, validate Kiali toolset configuration */
    if slices.Contains(m.StaticConfig.Toolsets, "kiali") {
        cfg, ok := m.StaticConfig.GetToolsetConfig("kiali")
        if !ok {
            return fmt.Errorf("kiali-url is required when kiali tools are enabled")
        }
        if err := cfg.Validate(); err != nil {
            // Normalize error message for missing URL to match expected UX/tests
            if strings.Contains(err.Error(), "kiali-url is required") {
                return fmt.Errorf("kiali-url is required when kiali tools are enabled")
            }
            return fmt.Errorf("invalid kiali configuration: %w", err)
        }
    }
	return nil
}

func (m *MCPServerOptions) Run() error {
	klog.V(1).Info("Starting kubernetes-mcp-server")
	klog.V(1).Infof(" - Config: %s", m.ConfigPath)
	klog.V(1).Infof(" - Toolsets: %s", strings.Join(m.StaticConfig.Toolsets, ", "))
	klog.V(1).Infof(" - ListOutput: %s", m.StaticConfig.ListOutput)
	klog.V(1).Infof(" - Read-only mode: %t", m.StaticConfig.ReadOnly)
	klog.V(1).Infof(" - Disable destructive tools: %t", m.StaticConfig.DisableDestructive)

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

	mcpServer, err := mcp.NewServer(mcp.Configuration{StaticConfig: m.StaticConfig})
	if err != nil {
		return fmt.Errorf("failed to initialize MCP server: %w", err)
	}
	defer mcpServer.Close()

	if m.StaticConfig.Port != "" {
		ctx := context.Background()
		return internalhttp.Serve(ctx, mcpServer, m.StaticConfig, oidcProvider, httpClient)
	}

	if err := mcpServer.ServeStdio(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}
