// Package korrel8r adapts the upstream Korrel8r MCP tools to this server.
package korrel8r

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/obs/auth"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	"github.com/google/jsonschema-go/jsonschema"
	k8rauth "github.com/korrel8r/korrel8r/pkg/api/auth"
	k8rmcp "github.com/korrel8r/korrel8r/pkg/mcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/client-go/rest"
)

const ToolsetName = "observability/korrel8r"

// toolPrefix disambiguates the upstream tool names from other toolsets.
const toolPrefix = "korrel8r_"

// defaultTimeout is a backstop against a hung connection to the Korrel8r
// backend. It is deliberately longer than Korrel8r's own request deadline
// (if one is configured on the backend) so
// that the server's timeout, error, or truncated partial result wins over a
// client-side cut-off. Raise it with the timeout option if the backend is
// tuned for longer traversals.
const defaultTimeout = 2 * time.Minute

// Config selects the Korrel8r REST server, authentication source and request timeout.
type Config struct {
	URL      string        `toml:"url"`
	AuthMode auth.AuthMode `toml:"auth_mode,omitempty"`
	// Timeout bounds a single REST call, e.g. "5m". Defaults to defaultTimeout.
	Timeout time.Duration `toml:"timeout,omitempty"`
}

func (c *Config) getAuthMode() auth.AuthMode {
	if c.AuthMode == "" {
		return auth.AuthModeHeader
	}
	return c.AuthMode
}

func (c *Config) getTimeout() time.Duration {
	if c.Timeout <= 0 {
		return defaultTimeout
	}
	return c.Timeout
}

// parseURL returns the validated backend URL.
func (c *Config) parseURL() (*url.URL, error) {
	u, err := url.Parse(c.URL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, fmt.Errorf("invalid korrel8r url %q: an http(s) URL is required", c.URL)
	}
	// The upstream client concatenates baseURL and its API paths as strings.
	// Queries, fragments and userinfo would be misplaced or leak credentials.
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(c.URL, "#") {
		return nil, fmt.Errorf("invalid korrel8r url %q: userinfo, query and fragment are not supported", c.URL)
	}
	return u, nil
}

func (c *Config) Validate() error {
	if c.AuthMode != "" {
		if _, err := auth.ParseAuthMode(string(c.AuthMode)); err != nil {
			return err
		}
	}
	if c.Timeout < 0 {
		return fmt.Errorf("invalid korrel8r timeout %v: must not be negative", c.Timeout)
	}
	_, err := c.parseURL()
	return err
}

func init() {
	config.RegisterToolsetConfig(ToolsetName, func(_ context.Context, p toml.Primitive, md toml.MetaData) (config.ExtendedConfig, error) {
		var cfg Config
		if err := md.PrimitiveDecode(p, &cfg); err != nil {
			return nil, err
		}
		return &cfg, nil
	})
	toolsets.Register(&Toolset{})
}

// Toolset adapts the upstream Korrel8r tools. The zero value is ready to use.
type Toolset struct {
	mu      sync.Mutex
	backend *backend // Most recently used backend, shared by all callers.
}

var (
	_ api.Toolset           = (*Toolset)(nil)
	_ config.ExtendedConfig = (*Config)(nil)
)

func (*Toolset) GetName() string { return ToolsetName }
func (*Toolset) GetDescription() string {
	return "Correlate Kubernetes resources with logs, metrics, alerts, and traces using Korrel8r."
}
func (*Toolset) GetPrompts(context.Context, api.ToolsetContext) []api.ServerPrompt     { return nil }
func (*Toolset) GetResources(context.Context, api.ToolsetContext) []api.ServerResource { return nil }
func (*Toolset) GetResourceTemplates(context.Context, api.ToolsetContext) []api.ServerResourceTemplate {
	return nil
}

// GetTools mechanically converts the upstream tool definitions.
// Upstream remains the sole owner of schemas, descriptions and handlers.
func (ts *Toolset) GetTools(_ context.Context, _ api.ToolsetContext) []api.ServerTool {
	tools := make([]api.ServerTool, 0, len(k8rmcp.ToolNames))
	for _, name := range k8rmcp.ToolNames {
		t := bind(nil, name).Tool
		tools = append(tools, api.ServerTool{
			Tool: api.Tool{
				Name:         toolPrefix + name,
				Description:  toolNameReplacer.Replace(t.Description),
				Annotations:  annotations(t.Annotations),
				InputSchema:  schema(t.InputSchema),
				OutputSchema: schema(t.OutputSchema),
			},
			RBAC:         api.RBACNone(),
			ClusterAware: new(false),
			Handler:      ts.handler(name),
		})
	}
	return tools
}

// bind returns the upstream binding for name. Each call allocates a fresh tool
// definition, schemas and a handler bound to client.
func bind(client *k8rmcp.Client, name string) k8rmcp.Binding {
	binding, ok := k8rmcp.BindTool(client, name)
	if !ok {
		panic("unknown korrel8r tool " + name) // Names come from upstream.ToolNames.
	}
	return binding
}

// Only quoted tool references are rewritten; ordinary prose is left alone.
var toolNameReplacer = func() *strings.Replacer {
	replacements := make([]string, 0, len(k8rmcp.ToolNames)*2)
	for _, name := range k8rmcp.ToolNames {
		replacements = append(replacements, "'"+name+"'", "'"+toolPrefix+name+"'")
	}
	return strings.NewReplacer(replacements...)
}()

func annotations(a *mcp.ToolAnnotations) api.ToolAnnotations {
	if a == nil {
		return api.ToolAnnotations{}
	}
	return api.ToolAnnotations{
		Title:           a.Title,
		ReadOnlyHint:    new(a.ReadOnlyHint),
		DestructiveHint: a.DestructiveHint,
		IdempotentHint:  new(a.IdempotentHint),
		OpenWorldHint:   a.OpenWorldHint,
	}
}

// schema converts an upstream schema, typed as any by the MCP SDK, and renames
// tool references in its property descriptions.
func schema(v any) *jsonschema.Schema {
	s, _ := v.(*jsonschema.Schema)
	if s == nil {
		return nil
	}
	for _, property := range s.Properties {
		property.Description = toolNameReplacer.Replace(property.Description)
	}
	return s
}

// handler calls the named upstream tool with the caller's identity.
// Configuration problems are reported as tool errors, not transport errors, so
// the model can recover (e.g. by telling the user what to configure).
func (ts *Toolset) handler(name string) api.ToolHandlerFunc {
	return func(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
		ctx := params.Context
		b, token, err := ts.getBackend(params)
		if err != nil {
			// An unconfigured toolset is a user configuration state, not a server fault.
			klogutil.FromContext(ctx).V(1).Info("Korrel8r client unavailable", "tool", name, "error", err.Error())
			return api.NewToolCallResult("", err), nil
		}
		binding := bind(b.client, name)
		ctx = k8rauth.WithToken(ctx, token)
		args, err := json.Marshal(params.GetArguments())
		if err != nil {
			return nil, fmt.Errorf("marshal korrel8r arguments: %w", err)
		}
		// Upstream reports tool failures in the result, not as an error.
		result, err := binding.Handler(ctx,
			&mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: name, Arguments: args}})
		if err != nil {
			return nil, err
		}
		rewriteResult(result)
		return convertResult(result)
	}
}

// rewriteResult renames tool references in help results. Results of the other
// tools are cluster data, not guidance, and are passed through unchanged.
func rewriteResult(result *mcp.CallToolResult) {
	if result == nil || result.IsError {
		return
	}
	// Structured content is freshly allocated per call, safe to modify in place.
	// Upstream returns one of these by value or by pointer; accept both so a
	// change of shape cannot silently disable the rewrite.
	switch content := result.StructuredContent.(type) {
	case k8rmcp.HelpResult:
		content.Documentation = toolNameReplacer.Replace(content.Documentation)
		result.StructuredContent = content
	case *k8rmcp.HelpResult:
		content.Documentation = toolNameReplacer.Replace(content.Documentation)
	default:
		return
	}
	// Keep the JSON text in step with the structured content, as upstream does.
	// Upstream text is the sole source of guidance: leave it if it cannot encode.
	if text, err := json.Marshal(result.StructuredContent); err == nil {
		result.Content = []mcp.Content{&mcp.TextContent{Text: string(text)}}
	}
}

// The client and its credential-free transport are shared across callers.
// Each request supplies its own bearer token through its context.
type backend struct {
	key       backendKey
	transport http.RoundTripper
	client    *k8rmcp.Client
}

// backendKey holds every input that is fixed when a backend is built.
// Options read live on each request (require_tls, max_backend_response_bytes)
// are read from the server config, which is part of the key.
type backendKey struct {
	serverConfig  *config.Config
	url           string
	timeout       time.Duration
	tlsMinVersion string
	cipherSuites  string
	caData        string
	caFile        string
}

// getBackend returns the backend for the current configuration and the
// caller's bearer token. Only the most recently used backend is kept: the
// configuration rarely changes, and a replaced backend's idle connections are closed.
func (ts *Toolset) getBackend(params api.ToolHandlerParams) (*backend, string, error) {
	c, ok := params.GetToolsetConfig(ToolsetName)
	if !ok {
		return nil, "", fmt.Errorf("korrel8r endpoint not configured: set [toolset_configs.%q].url", ToolsetName)
	}
	cfg, ok := c.(*Config)
	if !ok {
		return nil, "", fmt.Errorf("invalid korrel8r configuration type %T", c)
	}
	u, err := cfg.parseURL()
	if err != nil {
		return nil, "", err
	}
	if params.Config == nil {
		return nil, "", fmt.Errorf("no server config available")
	}
	restConfig := params.RESTConfig()
	if restConfig == nil {
		return nil, "", fmt.Errorf("no REST config available")
	}
	token, err := auth.ReadToken(params.Context, restConfig, cfg.getAuthMode())
	if err != nil {
		return nil, "", err
	}
	if token == "" && cfg.getAuthMode() == auth.AuthModeKubeConfig {
		klogutil.FromContext(params.Context).V(1).Info("Korrel8r kubeconfig auth has no bearer token; client certificates and exec plugins are not forwarded")
	}
	key := backendKey{
		serverConfig:  params.Config,
		url:           strings.TrimRight(u.String(), "/"),
		timeout:       cfg.getTimeout(),
		tlsMinVersion: params.Config.TLSMinVersion.Get(),
		cipherSuites:  strings.Join(params.Config.TLSCipherSuites.Get(), ","),
		caData:        string(restConfig.CAData),
		caFile:        restConfig.CAFile,
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.backend != nil && ts.backend.key == key {
		return ts.backend, token, nil
	}
	b, err := newBackend(params.Context, key, restConfig, u.Scheme == "https")
	if err != nil {
		return nil, "", err
	}
	if ts.backend != nil {
		closeIdleConnections(ts.backend.transport)
	}
	ts.backend = b
	return b, token, nil
}

func newBackend(ctx context.Context, key backendKey, restConfig *rest.Config, useTLS bool) (*backend, error) {
	// Build a transport without credentials: the kubeconfig auth mode with
	// the bearer token removed keeps only the TLS policy and CA trust.
	noCredentials := rest.CopyConfig(restConfig)
	noCredentials.BearerToken, noCredentials.BearerTokenFile = "", ""
	transport, err := auth.BuildRoundTripper(ctx, key.serverConfig, noCredentials, auth.AuthModeKubeConfig, useTLS, false)
	if err != nil {
		return nil, err
	}
	rt := kubernetes.NewBackendResponseRoundTripper(k8rauth.Wrap(transport), kubernetes.LiveBackendLimit(key.serverConfig))
	httpClient := &http.Client{Transport: rt, Timeout: key.timeout}
	client := k8rmcp.NewClient(key.url, httpClient)
	// The transport enforces the live max_backend_response_bytes, including
	// "0 = unlimited". Upstream treats 0 as its own 8 MiB default, so disable
	// its limit instead. It reads limit+1 bytes, so MaxInt64 would overflow.
	client.MaxResponseBytes = math.MaxInt64 - 1
	return &backend{key: key, transport: transport, client: client}, nil
}

// closeIdleConnections closes idle connections of a transport built by
// auth.BuildRoundTripper without credentials: a TLS-enforcing wrapper
// around an *http.Transport.
func closeIdleConnections(rt http.RoundTripper) {
	if t, ok := rt.(*config.TLSEnforcingTransport); ok {
		rt = t.Base
	}
	if c, ok := rt.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}

// convertResult adapts a Korrel8r MCP SDK result to this server's tool result.
// It preserves structured content, copies text blocks, and turns upstream
// isError results into prefixed errors for the caller. Korrel8r returns only text blocks.
func convertResult(result *mcp.CallToolResult) (*api.ToolCallResult, error) {
	if result == nil {
		return nil, fmt.Errorf("korrel8r returned no result")
	}
	out := &api.ToolCallResult{StructuredContent: result.StructuredContent}
	var message strings.Builder
	for i, content := range result.Content {
		text, ok := content.(*mcp.TextContent)
		if !ok {
			return nil, fmt.Errorf("unsupported korrel8r content type %T", content)
		}
		out.ContentBlocks = append(out.ContentBlocks, api.NewTextToolContent(text.Text))
		if result.IsError {
			if i > 0 {
				message.WriteByte('\n')
			}
			message.WriteString(text.Text)
		}
	}
	if result.IsError {
		errorText := message.String()
		// Upstream REST errors already name korrel8r.
		if !strings.HasPrefix(errorText, "korrel8r") {
			errorText = "korrel8r: " + errorText
		}
		out.Error = errors.New(errorText)
	}
	return out, nil
}
