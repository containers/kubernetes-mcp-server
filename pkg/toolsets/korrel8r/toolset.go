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
	"os"
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
	upstream "github.com/korrel8r/korrel8r/pkg/mcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
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
	tools := make([]api.ServerTool, 0, len(upstream.ToolNames))
	for _, name := range upstream.ToolNames {
		t := bind(nil, name).Tool
		description := toolNameReplacer.Replace(t.Description)
		if name == upstream.GetConsole || name == upstream.ShowInConsole {
			description += " Optional OpenShift integration; requires a console connected as this user to the same Korrel8r service."
		}
		tools = append(tools, api.ServerTool{
			Tool: api.Tool{
				Name:         toolPrefix + name,
				Description:  description,
				Annotations:  annotations(t.Annotations),
				InputSchema:  schema(t.InputSchema),
				OutputSchema: schema(t.OutputSchema),
			},
			RBAC:         api.RBACNone(),
			ClusterAware: ptr.To(false),
			Handler:      ts.handler(name),
		})
	}
	return tools
}

// bind returns the upstream binding for name. Each call allocates a fresh tool
// definition, schemas and a handler bound to client.
func bind(client *upstream.Client, name string) upstream.Binding {
	binding, ok := upstream.BindTool(client, name)
	if !ok {
		panic("unknown korrel8r tool " + name) // Names come from upstream.ToolNames.
	}
	return binding
}

// Only quoted tool references are rewritten; ordinary prose is left alone.
var toolNameReplacer = func() *strings.Replacer {
	replacements := make([]string, 0, len(upstream.ToolNames)*2)
	for _, name := range upstream.ToolNames {
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
		ReadOnlyHint:    ptr.To(a.ReadOnlyHint),
		DestructiveHint: a.DestructiveHint,
		IdempotentHint:  ptr.To(a.IdempotentHint),
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

// instructions is the static upstream instructions tool. It needs no client.
var instructions = bind(nil, upstream.InstructionsTool)

// handler calls the named upstream tool with the caller's identity.
// Configuration problems are reported as tool errors, not transport errors, so
// the model can recover (e.g. by telling the user what to configure).
func (ts *Toolset) handler(name string) api.ToolHandlerFunc {
	return func(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
		binding := instructions
		ctx := params.Context
		// The upstream instructions tool is static. It must work even when
		// Korrel8r is not configured or the backend is unavailable.
		if name != upstream.InstructionsTool {
			b, token, err := ts.getBackend(params)
			if err != nil {
				// An unconfigured toolset is a user configuration state, not a server fault.
				klogutil.FromContext(ctx).V(1).Info("Korrel8r client unavailable", "tool", name, "error", err.Error())
				return api.NewToolCallResult("", err), nil
			}
			binding = b.bindings[name]
			ctx = context.WithValue(ctx, tokenKey{}, token)
		}
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

// rewriteResult renames tool references in results that document other tools:
// instructions and help. Results of the other tools are cluster data, not
// guidance, and are passed through unchanged.
func rewriteResult(result *mcp.CallToolResult) {
	if result == nil || result.IsError {
		return
	}
	// Structured content is freshly allocated per call, safe to modify in place.
	// Upstream returns one of these by value or by pointer; accept both so a
	// change of shape cannot silently disable the rewrite.
	switch content := result.StructuredContent.(type) {
	case upstream.InstructionsResult:
		content.Instructions = rewriteInstructions(content.Instructions)
		result.StructuredContent = content
	case *upstream.InstructionsResult:
		content.Instructions = rewriteInstructions(content.Instructions)
	case upstream.HelpResult:
		content.Documentation = toolNameReplacer.Replace(content.Documentation)
		result.StructuredContent = content
	case *upstream.HelpResult:
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

// Upstream suggests the console for generic questions. Correlation is the
// primary workflow; console integration only applies when explicitly requested.
func rewriteInstructions(instructions string) string {
	const consoleFirst = `If the user refers to a console, use 'get_console' and 'show_in_console' to read and update it.
If the user asks "what am I looking at?", "what is this?" or says "show me ..." or "display ..." they may be referring to the console.

`
	instructions = strings.Replace(instructions, consoleFirst, "", 1)
	return strings.TrimSpace(toolNameReplacer.Replace(instructions)) + "\n\n" +
		"On OpenShift, if the user explicitly refers to a connected OpenShift console, use 'korrel8r_get_console' and 'korrel8r_show_in_console'. These optional tools require a console connected as the same user to the same Korrel8r service."
}

// backend is a Korrel8r REST client and its tool bindings. It is shared by
// all callers with the same configuration so that connections are reused and
// tool schemas are built once. The caller's bearer token is not part of the
// backend: it is passed in the request context and added to each request.
type backend struct {
	key       backendKey
	transport http.RoundTripper // Credential-free transport from auth.BuildRoundTripper.
	bindings  map[string]upstream.Binding
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

// tokenKey is the context key for the caller's bearer token.
type tokenKey struct{}

// bearerFromContext adds the caller's bearer token from the request context.
type bearerFromContext struct{ next http.RoundTripper }

func (t bearerFromContext) RoundTrip(req *http.Request) (*http.Response, error) {
	if token, _ := req.Context().Value(tokenKey{}).(string); token != "" {
		req = req.Clone(req.Context())
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return t.next.RoundTrip(req)
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
	token, err := readToken(params.Context, restConfig, cfg.getAuthMode())
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
	rt := kubernetes.NewBackendResponseRoundTripper(bearerFromContext{next: transport}, kubernetes.LiveBackendLimit(key.serverConfig))
	httpClient := &http.Client{Transport: rt, Timeout: key.timeout}
	client := upstream.NewClient(key.url, httpClient)
	// The transport enforces the live max_backend_response_bytes, including
	// "0 = unlimited". Upstream treats 0 as its own 8 MiB default, so disable
	// its limit instead. It reads limit+1 bytes, so MaxInt64 would overflow.
	client.MaxResponseBytes = math.MaxInt64 - 1
	bindings := make(map[string]upstream.Binding, len(upstream.ToolNames))
	for _, name := range upstream.ToolNames {
		bindings[name] = bind(client, name)
	}
	return &backend{key: key, transport: transport, bindings: bindings}, nil
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

// readToken returns the caller's bearer token for authMode, or "" if there is
// none. It matches the token selection of auth.BuildRoundTripper. Only bearer
// tokens are supported: client certificates and exec plugins produce none.
func readToken(ctx context.Context, restConfig *rest.Config, authMode auth.AuthMode) (string, error) {
	switch authMode {
	case auth.AuthModeKubeConfig:
		if restConfig.BearerToken != "" {
			return restConfig.BearerToken, nil
		}
		if restConfig.BearerTokenFile != "" {
			token, err := os.ReadFile(restConfig.BearerTokenFile)
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(string(token)), nil
		}
		return "", nil
	case auth.AuthModeHeader:
		header, _ := ctx.Value(kubernetes.OAuthAuthorizationHeader).(string)
		token := strings.TrimSpace(header)
		if parts := strings.Fields(header); len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			token = parts[1]
		}
		if token == "" {
			klogutil.LogWarn(klogutil.FromContext(ctx), "no bearer token found in request context authorization header")
		}
		return token, nil
	default:
		return "", fmt.Errorf("unsupported auth mode: %s", authMode)
	}
}

// convertResult converts an upstream result. Korrel8r tools only return text.
func convertResult(result *mcp.CallToolResult) (*api.ToolCallResult, error) {
	if result == nil {
		return nil, fmt.Errorf("korrel8r returned no result")
	}
	out := &api.ToolCallResult{StructuredContent: result.StructuredContent}
	var messages []string
	for _, content := range result.Content {
		text, ok := content.(*mcp.TextContent)
		if !ok {
			return nil, fmt.Errorf("unsupported korrel8r content type %T", content)
		}
		out.ContentBlocks = append(out.ContentBlocks, api.NewTextToolContent(text.Text))
		messages = append(messages, text.Text)
	}
	if result.IsError {
		message := strings.Join(messages, "\n")
		// Upstream REST errors already name korrel8r.
		if !strings.HasPrefix(message, "korrel8r") {
			message = "korrel8r: " + message
		}
		out.Error = errors.New(message)
	}
	return out, nil
}
