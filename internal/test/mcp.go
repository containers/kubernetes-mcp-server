package test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// McpClientOption configures McpClient creation
type McpClientOption interface {
	apply(*mcpClientConfig)
}

type mcpClientConfig struct {
	headers              map[string]string
	clientInfo           *mcp.Implementation
	endpoint             string
	allowConnectionError bool
}

// httpHeaderOption sets custom HTTP headers
type httpHeaderOption struct {
	headers map[string]string
}

func (o httpHeaderOption) apply(c *mcpClientConfig) {
	if c.headers == nil {
		c.headers = make(map[string]string)
	}
	for k, v := range o.headers {
		c.headers[k] = v
	}
}

// WithHTTPHeaders sets custom HTTP headers for the MCP client transport
func WithHTTPHeaders(headers map[string]string) McpClientOption {
	return httpHeaderOption{headers: headers}
}

// clientInfoOption sets custom client info
type clientInfoOption struct {
	info mcp.Implementation
}

func (o clientInfoOption) apply(c *mcpClientConfig) {
	c.clientInfo = &o.info
}

// WithClientInfo sets custom MCP client info for initialization
func WithClientInfo(name, version string) McpClientOption {
	return clientInfoOption{info: mcp.Implementation{Name: name, Version: version}}
}

// WithEmptyClientInfo sets empty MCP client info for initialization
func WithEmptyClientInfo() McpClientOption {
	return clientInfoOption{info: mcp.Implementation{}}
}

// endpointOption sets a custom endpoint URL instead of using httptest.Server
type endpointOption struct {
	endpoint string
}

func (o endpointOption) apply(c *mcpClientConfig) {
	c.endpoint = o.endpoint
}

// WithEndpoint sets a custom endpoint URL for the MCP client.
// When set, no httptest.Server will be created and the provided URL will be used directly.
// The URL should include the full path (e.g., "http://localhost:8080/mcp").
func WithEndpoint(endpoint string) McpClientOption {
	return endpointOption{endpoint: endpoint}
}

// allowConnectionErrorOption allows connection errors without failing the test
type allowConnectionErrorOption struct{}

func (o allowConnectionErrorOption) apply(c *mcpClientConfig) {
	c.allowConnectionError = true
}

// WithAllowConnectionError allows connection errors without failing the test.
// When set, connection failures will result in a nil Session instead of a test failure.
// Useful for testing authentication/authorization scenarios where connection rejection is expected.
func WithAllowConnectionError() McpClientOption {
	return allowConnectionErrorOption{}
}

// headerRoundTripper injects HTTP headers into requests
type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range h.headers {
		req.Header.Set(k, v)
	}
	return h.base.RoundTrip(req)
}

// CapturedNotification represents a captured MCP notification for testing
type CapturedNotification struct {
	Method string
	Params any
}

type McpClient struct {
	ctx              context.Context
	testServer       *httptest.Server
	client           *mcp.Client
	Session          *mcp.ClientSession
	InitializeResult *mcp.InitializeResult
	notifications    *NotificationCapture
}

// NewMcpClient creates a new MCP test client.
//
// When an http.Handler is provided, an httptest.Server is created automatically.
// Alternatively, use WithEndpoint() option to connect to an existing server URL.
//
// Example with handler:
//
//	client := test.NewMcpClient(t, myHandler)
//
// Example with endpoint:
//
//	client := test.NewMcpClient(t, nil, test.WithEndpoint("http://localhost:8080/mcp"))
func NewMcpClient(t *testing.T, mcpHttpServer http.Handler, options ...McpClientOption) *McpClient {
	cfg := &mcpClientConfig{
		clientInfo: &mcp.Implementation{Name: "test", Version: "1.33.7"},
	}
	for _, opt := range options {
		opt.apply(cfg)
	}

	// Validate configuration
	if mcpHttpServer == nil && cfg.endpoint == "" {
		require.Fail(t, "Either mcpHttpServer or WithEndpoint() option must be provided")
		return nil
	}

	// Initialize notification capture immediately. Notifications are always captured
	// from the moment the client connects. Use StartCapturingNotifications() to get
	// the capture object for waiting on specific notifications in tests.
	ret := &McpClient{
		ctx: t.Context(),
		notifications: &NotificationCapture{
			notifications: make([]*CapturedNotification, 0),
			signal:        make(chan struct{}, 1),
		},
	}

	// Determine the endpoint URL
	var endpoint string
	if cfg.endpoint != "" {
		// Use provided endpoint directly
		endpoint = cfg.endpoint
	} else {
		// Create httptest.Server from handler
		ret.testServer = httptest.NewServer(mcpHttpServer)
		endpoint = ret.testServer.URL + "/mcp"
	}

	// Create HTTP client with custom headers if provided
	httpClient := http.DefaultClient
	if len(cfg.headers) > 0 {
		httpClient = &http.Client{
			Transport: &headerRoundTripper{
				base:    http.DefaultTransport,
				headers: cfg.headers,
			},
		}
	}

	// Create go-sdk client with notification handlers
	clientOptions := &mcp.ClientOptions{
		ToolListChangedHandler: func(_ context.Context, req *mcp.ToolListChangedRequest) {
			ret.notifications.capture(&CapturedNotification{
				Method: "notifications/tools/list_changed",
				Params: req.Params,
			})
		},
		PromptListChangedHandler: func(_ context.Context, req *mcp.PromptListChangedRequest) {
			ret.notifications.capture(&CapturedNotification{
				Method: "notifications/prompts/list_changed",
				Params: req.Params,
			})
		},
		ResourceListChangedHandler: func(_ context.Context, req *mcp.ResourceListChangedRequest) {
			ret.notifications.capture(&CapturedNotification{
				Method: "notifications/resources/list_changed",
				Params: req.Params,
			})
		},
		LoggingMessageHandler: func(_ context.Context, req *mcp.LoggingMessageRequest) {
			ret.notifications.capture(&CapturedNotification{
				Method: "notifications/message",
				Params: req.Params,
			})
		},
	}

	ret.client = mcp.NewClient(cfg.clientInfo, clientOptions)

	// Create transport with StreamableClientTransport
	transport := &mcp.StreamableClientTransport{
		Endpoint:   endpoint,
		HTTPClient: httpClient,
	}

	var err error
	ret.Session, err = ret.client.Connect(t.Context(), transport, nil)
	if err != nil {
		if cfg.allowConnectionError {
			// Connection error is allowed, return client with nil session
			ret.Session = nil
			return ret
		}
		require.NoError(t, err, "Expected no error connecting MCP client")
	}

	if ret.Session != nil {
		ret.InitializeResult = ret.Session.InitializeResult()
	}
	return ret
}

func (m *McpClient) Close() {
	if m.Session != nil {
		_ = m.Session.Close()
	}
	if m.testServer != nil {
		m.testServer.Close()
	}
}

// CallTool helper function to call a tool by name with arguments
func (m *McpClient) CallTool(name string, args map[string]any) (*mcp.CallToolResult, error) {
	return m.Session.CallTool(m.ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
}

// ListTools helper function to list available tools
func (m *McpClient) ListTools() (*mcp.ListToolsResult, error) {
	return m.Session.ListTools(m.ctx, &mcp.ListToolsParams{})
}

// ListPrompts helper function to list available prompts
func (m *McpClient) ListPrompts() (*mcp.ListPromptsResult, error) {
	return m.Session.ListPrompts(m.ctx, &mcp.ListPromptsParams{})
}

// GetPrompt helper function to get a prompt by name
func (m *McpClient) GetPrompt(name string, arguments map[string]string) (*mcp.GetPromptResult, error) {
	return m.Session.GetPrompt(m.ctx, &mcp.GetPromptParams{
		Name:      name,
		Arguments: arguments,
	})
}

// SetLoggingLevel sets the logging level on the server
func (m *McpClient) SetLoggingLevel(level mcp.LoggingLevel) error {
	return m.Session.SetLoggingLevel(m.ctx, &mcp.SetLoggingLevelParams{
		Level: level,
	})
}

// NotificationCapture captures MCP notifications for testing.
// Use StartCapturingNotifications to begin capturing, then RequireNotification to retrieve.
type NotificationCapture struct {
	mu            sync.RWMutex
	notifications []*CapturedNotification
	signal        chan struct{} // signals when new notifications arrive
}

func (c *NotificationCapture) capture(n *CapturedNotification) {
	c.mu.Lock()
	c.notifications = append(c.notifications, n)
	c.mu.Unlock()
	// Signal that a new notification arrived (non-blocking)
	select {
	case c.signal <- struct{}{}:
	default:
	}
}

// StartCapturingNotifications returns the notification capture for waiting on notifications.
// The notifications are always being captured; this just returns the capture object.
func (m *McpClient) StartCapturingNotifications() *NotificationCapture {
	return m.notifications
}

// RequireNotification waits for a notification matching the specified method and fails the test if not received.
// Iterates through all captured notifications looking for a match, waiting for new ones if needed.
// The method parameter specifies which notification method to wait for (e.g., "notifications/tools/list_changed").
//
// Timeout recommendations:
//   - 2 seconds: For immediate notifications like log messages after tool calls
//   - 5 seconds: For notifications involving file system or cluster state changes (kubeconfig, API groups)
func (c *NotificationCapture) RequireNotification(t *testing.T, timeout time.Duration, method string) *CapturedNotification {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	checked := 0
	for {
		// Check all notifications we haven't checked yet
		c.mu.RLock()
		for i := checked; i < len(c.notifications); i++ {
			if c.notifications[i].Method == method {
				n := c.notifications[i]
				c.mu.RUnlock()
				return n
			}
		}
		checked = len(c.notifications)
		c.mu.RUnlock()

		// Wait for new notifications or timeout
		select {
		case <-c.signal:
			// New notification arrived, loop and check it
		case <-ctx.Done():
			require.Fail(t, "timeout waiting for MCP notification", "method: %s", method)
			return nil
		}
	}
}

// LogNotification represents a parsed MCP logging notification.
// Used for asserting log messages sent to MCP clients during error handling.
type LogNotification struct {
	// Level is the log severity level (debug, info, notice, warning, error, critical, alert, emergency)
	Level string
	// Logger is the name of the logger that generated the message
	Logger string
	// Data contains the log message content
	Data string
}

// parseLogNotification extracts log information from a CapturedNotification.
// Returns nil if the notification is not a valid logging notification.
func parseLogNotification(notification *CapturedNotification) *LogNotification {
	if notification == nil {
		return nil
	}

	// The Params field should be *mcp.LoggingMessageParams
	if params, ok := notification.Params.(*mcp.LoggingMessageParams); ok {
		// Convert Data to string
		var dataStr string
		switch v := params.Data.(type) {
		case string:
			dataStr = v
		default:
			dataBytes, _ := json.Marshal(v)
			dataStr = string(dataBytes)
		}
		return &LogNotification{
			Level:  string(params.Level),
			Logger: params.Logger,
			Data:   dataStr,
		}
	}

	return nil
}

// RequireLogNotification waits for a logging notification and returns it parsed.
// Filters for "notifications/message" method and fails the test if not received within timeout.
//
// Timeout recommendations:
//   - 2 seconds: Standard timeout for log notifications after tool calls (recommended default)
func (c *NotificationCapture) RequireLogNotification(t *testing.T, timeout time.Duration) *LogNotification {
	notification := c.RequireNotification(t, timeout, "notifications/message")
	logNotification := parseLogNotification(notification)
	require.NotNil(t, logNotification, "failed to parse log notification")
	return logNotification
}

// RequireNoLogNotification asserts that no logging notification is received within the given timeout.
// Use this to verify that non-Kubernetes errors do not produce MCP log notifications.
func (c *NotificationCapture) RequireNoLogNotification(t *testing.T, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		c.mu.RLock()
		for _, n := range c.notifications {
			if n.Method == "notifications/message" {
				c.mu.RUnlock()
				require.Fail(t, "unexpected log notification received", "notification: %v", n)
				return
			}
		}
		c.mu.RUnlock()

		select {
		case <-c.signal:
			// New notification arrived, check it
		case <-ctx.Done():
			// Timeout with no log notification — success
			return
		}
	}
}
