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
	"github.com/containers/kubernetes-mcp-server/pkg/mcp"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
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
`))
)

type MCPServerOptions struct {
	Version              bool
	LogLevel             int
	Port                 string
	SSEPort              int
	HttpPort             int
	SSEBaseUrl           string
	Kubeconfig           string
	KubeContext          string
	Profile              string
	ListOutput           string
	ReadOnly             bool
	DisableDestructive   bool
	RequireOAuth         bool
	OAuthAudience        string
	ValidateToken        bool
	AuthorizationURL     string
	CertificateAuthority string
	ServerURL            string

	ConfigPath   string
	StaticConfig *config.StaticConfig

	genericiooptions.IOStreams
}

func NewMCPServerOptions(streams genericiooptions.IOStreams) *MCPServerOptions {
	return &MCPServerOptions{
		IOStreams:    streams,
		Profile:      "full",
		ListOutput:   "table",
		StaticConfig: &config.StaticConfig{},
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

	cmd.Flags().BoolVar(&o.Version, "version", o.Version, "Print version information and quit")
	cmd.Flags().IntVar(&o.LogLevel, "log-level", o.LogLevel, "Set the log level (from 0 to 9)")
	cmd.Flags().StringVar(&o.ConfigPath, "config", o.ConfigPath, "Path of the config file. Each profile has its set of defaults.")
	cmd.Flags().IntVar(&o.SSEPort, "sse-port", o.SSEPort, "Start a SSE server on the specified port")
	cmd.Flag("sse-port").Deprecated = "Use --port instead"
	cmd.Flags().IntVar(&o.HttpPort, "http-port", o.HttpPort, "Start a streamable HTTP server on the specified port")
	cmd.Flag("http-port").Deprecated = "Use --port instead"
	cmd.Flags().StringVar(&o.Port, "port", o.Port, "Start a streamable HTTP and SSE HTTP server on the specified port (e.g. 8080)")
	cmd.Flags().StringVar(&o.SSEBaseUrl, "sse-base-url", o.SSEBaseUrl, "SSE public base URL to use when sending the endpoint message (e.g. https://example.com)")
	cmd.Flags().StringVar(&o.Kubeconfig, "kubeconfig", o.Kubeconfig, "Path to the kubeconfig file to use for authentication")
	cmd.Flags().StringVar(&o.KubeContext, "kubecontext", o.Kubeconfig, "Context name from kube config")
	cmd.Flags().StringVar(&o.Profile, "profile", o.Profile, "MCP profile to use (one of: "+strings.Join(mcp.ProfileNames, ", ")+")")
	cmd.Flags().StringVar(&o.ListOutput, "list-output", o.ListOutput, "Output format for resource list operations (one of: "+strings.Join(output.Names, ", ")+"). Defaults to table.")
	cmd.Flags().BoolVar(&o.ReadOnly, "read-only", o.ReadOnly, "If true, only tools annotated with readOnlyHint=true are exposed")
	cmd.Flags().BoolVar(&o.DisableDestructive, "disable-destructive", o.DisableDestructive, "If true, tools annotated with destructiveHint=true are disabled")
	cmd.Flags().BoolVar(&o.RequireOAuth, "require-oauth", o.RequireOAuth, "If true, requires OAuth authorization as defined in the Model Context Protocol (MCP) specification. This flag is ignored if transport type is stdio")
	_ = cmd.Flags().MarkHidden("require-oauth")
	cmd.Flags().StringVar(&o.OAuthAudience, "oauth-audience", o.OAuthAudience, "OAuth audience for token claims validation. Optional. If not set, the audience is not validated. Only valid if require-oauth is enabled.")
	_ = cmd.Flags().MarkHidden("oauth-audience")
	cmd.Flags().BoolVar(&o.ValidateToken, "validate-token", o.ValidateToken, "If true, validates the token against the Kubernetes API Server using TokenReview. Optional. If not set, the token is not validated. Only valid if require-oauth is enabled.")
	_ = cmd.Flags().MarkHidden("validate-token")
	cmd.Flags().StringVar(&o.AuthorizationURL, "authorization-url", o.AuthorizationURL, "OAuth authorization server URL for protected resource endpoint. If not provided, the Kubernetes API server host will be used. Only valid if require-oauth is enabled.")
	_ = cmd.Flags().MarkHidden("authorization-url")
	cmd.Flags().StringVar(&o.ServerURL, "server-url", o.ServerURL, "Server URL of this application. Optional. If set, this url will be served in protected resource metadata endpoint and tokens will be validated with this audience. If not set, expected audience is kubernetes-mcp-server. Only valid if require-oauth is enabled.")
	_ = cmd.Flags().MarkHidden("server-url")
	cmd.Flags().StringVar(&o.CertificateAuthority, "certificate-authority", o.CertificateAuthority, "Certificate authority path to verify certificates. Optional. Only valid if require-oauth is enabled.")
	_ = cmd.Flags().MarkHidden("certificate-authority")

	return cmd
}

func (m *MCPServerOptions) Complete(cmd *cobra.Command) error {
	if m.ConfigPath != "" {
		cnf, err := config.ReadConfig(m.ConfigPath)
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
	if cmd.Flag("log-level").Changed {
		m.StaticConfig.LogLevel = m.LogLevel
	}
	if cmd.Flag("port").Changed {
		m.StaticConfig.Port = m.Port
	} else if cmd.Flag("sse-port").Changed {
		m.StaticConfig.Port = strconv.Itoa(m.SSEPort)
	} else if cmd.Flag("http-port").Changed {
		m.StaticConfig.Port = strconv.Itoa(m.HttpPort)
	}
	if cmd.Flag("sse-base-url").Changed {
		m.StaticConfig.SSEBaseURL = m.SSEBaseUrl
	}
	if cmd.Flag("kubeconfig").Changed {
		m.StaticConfig.KubeConfig = m.Kubeconfig
	}
	if cmd.Flag("kubecontext").Changed {
		m.StaticConfig.KubeContext = m.KubeContext
	}
	if cmd.Flag("list-output").Changed || m.StaticConfig.ListOutput == "" {
		m.StaticConfig.ListOutput = m.ListOutput
	}
	if cmd.Flag("read-only").Changed {
		m.StaticConfig.ReadOnly = m.ReadOnly
	}
	if cmd.Flag("disable-destructive").Changed {
		m.StaticConfig.DisableDestructive = m.DisableDestructive
	}
	if cmd.Flag("require-oauth").Changed {
		m.StaticConfig.RequireOAuth = m.RequireOAuth
	}
	if cmd.Flag("oauth-audience").Changed {
		m.StaticConfig.OAuthAudience = m.OAuthAudience
	}
	if cmd.Flag("validate-token").Changed {
		m.StaticConfig.ValidateToken = m.ValidateToken
	}
	if cmd.Flag("authorization-url").Changed {
		m.StaticConfig.AuthorizationURL = m.AuthorizationURL
	}
	if cmd.Flag("server-url").Changed {
		m.StaticConfig.ServerURL = m.ServerURL
	}
	if cmd.Flag("certificate-authority").Changed {
		m.StaticConfig.CertificateAuthority = m.CertificateAuthority
	}
}

func (m *MCPServerOptions) initializeLogging() {
	flagSet := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(flagSet)
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
	return nil
}

func (m *MCPServerOptions) Run() error {
	profile := mcp.ProfileFromString(m.Profile)
	if profile == nil {
		return fmt.Errorf("invalid profile name: %s, valid names are: %s", m.Profile, strings.Join(mcp.ProfileNames, ", "))
	}
	listOutput := output.FromString(m.StaticConfig.ListOutput)
	if listOutput == nil {
		return fmt.Errorf("invalid output name: %s, valid names are: %s", m.StaticConfig.ListOutput, strings.Join(output.Names, ", "))
	}
	klog.V(1).Info("Starting kubernetes-mcp-server")
	klog.V(1).Infof(" - Config: %s", m.ConfigPath)
	klog.V(1).Infof(" - Profile: %s", profile.GetName())
	klog.V(1).Infof(" - ListOutput: %s", listOutput.GetName())
	klog.V(1).Infof(" - Read-only mode: %t", m.StaticConfig.ReadOnly)
	klog.V(1).Infof(" - Disable destructive tools: %t", m.StaticConfig.DisableDestructive)

	if m.Version {
		_, _ = fmt.Fprintf(m.Out, "%s\n", version.Version)
		return nil
	}

	var oidcProvider *oidc.Provider
	if m.StaticConfig.AuthorizationURL != "" {
		ctx := context.Background()
		if m.StaticConfig.CertificateAuthority != "" {
			httpClient := &http.Client{}
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

	mcpServer, err := mcp.NewServer(mcp.Configuration{
		Profile:      profile,
		ListOutput:   listOutput,
		StaticConfig: m.StaticConfig,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize MCP server: %w", err)
	}
	defer mcpServer.Close()

	if m.StaticConfig.Port != "" {
		ctx := context.Background()
		return internalhttp.Serve(ctx, mcpServer, m.StaticConfig, oidcProvider)
	}

	if err := mcpServer.ServeStdio(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}
