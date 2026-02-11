package test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// McpClientOption configures McpClient creation
type McpClientOption interface {
	apply(*mcpClientConfig)
}

type mcpClientConfig struct {
	transportOptions []transport.StreamableHTTPCOption
	clientInfo       *mcp.Implementation
}

// transportOptionWrapper wraps transport.StreamableHTTPCOption
type transportOptionWrapper struct {
	opt transport.StreamableHTTPCOption
}

func (t transportOptionWrapper) apply(c *mcpClientConfig) {
	c.transportOptions = append(c.transportOptions, t.opt)
}

// WithTransport wraps a transport.StreamableHTTPCOption for use with NewMcpClient
func WithTransport(opt transport.StreamableHTTPCOption) McpClientOption {
	return transportOptionWrapper{opt}
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

// McpInitRequest returns a default MCP initialization request for backward compatibility
func McpInitRequest() mcp.InitializeRequest {
	initRequest := mcp.InitializeRequest{
		Request: mcp.Request{Method: "initialize"},
	}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{Name: "test", Version: "1.33.7"}
	return initRequest
}

type McpClient struct {
	ctx        context.Context
	testServer *httptest.Server
	*client.Client
	InitializeResult *mcp.InitializeResult
}

func NewMcpClient(t *testing.T, mcpHttpServer http.Handler, options ...McpClientOption) *McpClient {
	require.NotNil(t, mcpHttpServer, "McpHttpServer must be provided")

	cfg := &mcpClientConfig{
		clientInfo: &mcp.Implementation{Name: "test", Version: "1.33.7"},
	}
	for _, opt := range options {
		opt.apply(cfg)
	}

	var err error
	ret := &McpClient{ctx: t.Context()}
	ret.testServer = httptest.NewServer(mcpHttpServer)
	transportOpts := append(cfg.transportOptions, transport.WithContinuousListening())
	ret.Client, err = client.NewStreamableHttpClient(ret.testServer.URL+"/mcp", transportOpts...)
	require.NoError(t, err, "Expected no error creating MCP client")
	err = ret.Start(t.Context())
	require.NoError(t, err, "Expected no error starting MCP client")

	initRequest := mcp.InitializeRequest{
		Request: mcp.Request{Method: "initialize"},
	}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = *cfg.clientInfo

	ret.InitializeResult, err = ret.Initialize(t.Context(), initRequest)
	require.NoError(t, err, "Expected no error initializing MCP client")
	return ret
}

func (m *McpClient) Close() {
	if m.Client != nil {
		_ = m.Client.Close()
	}
	if m.testServer != nil {
		m.testServer.Close()
	}
}

// CallTool helper function to call a tool by name with arguments
func (m *McpClient) CallTool(name string, args map[string]any) (*mcp.CallToolResult, error) {
	callToolRequest := mcp.CallToolRequest{}
	callToolRequest.Params.Name = name
	callToolRequest.Params.Arguments = args
	return m.Client.CallTool(m.ctx, callToolRequest)
}

// NotificationCapture captures MCP notifications for testing.
// Use StartCapturingNotifications to begin capturing, then RequireNotification to retrieve.
type NotificationCapture struct {
	mu            sync.RWMutex
	notifications []*mcp.JSONRPCNotification
	signal        chan struct{} // signals when new notifications arrive
}

// StartCapturingNotifications begins capturing all MCP notifications.
// Must be called BEFORE the operation that triggers the notification.
func (m *McpClient) StartCapturingNotifications() *NotificationCapture {
	capture := &NotificationCapture{
		notifications: make([]*mcp.JSONRPCNotification, 0),
		signal:        make(chan struct{}, 1),
	}
	m.OnNotification(func(n mcp.JSONRPCNotification) {
		capture.mu.Lock()
		capture.notifications = append(capture.notifications, &n)
		capture.mu.Unlock()
		// Signal that a new notification arrived (non-blocking)
		select {
		case capture.signal <- struct{}{}:
		default:
		}
	})
	return capture
}

// RequireNotification waits for a notification matching the specified method and fails the test if not received.
// Iterates through all captured notifications looking for a match, waiting for new ones if needed.
// The method parameter specifies which notification method to wait for (e.g., "notifications/tools/list_changed").
//
// Timeout recommendations:
//   - 2 seconds: For immediate notifications like log messages after tool calls
//   - 5 seconds: For notifications involving file system or cluster state changes (kubeconfig, API groups)
func (c *NotificationCapture) RequireNotification(t *testing.T, timeout time.Duration, method string) *mcp.JSONRPCNotification {
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

// parseLogNotification extracts log information from an MCP notification.
// Returns nil if the notification is not a valid logging notification.
func parseLogNotification(notification *mcp.JSONRPCNotification) *LogNotification {
	if notification == nil {
		return nil
	}
	// The Params field contains the LoggingMessageParams via AdditionalFields
	paramsBytes, err := json.Marshal(notification.Params)
	if err != nil {
		return nil
	}
	var logParams struct {
		Level  string `json:"level"`
		Logger string `json:"logger"`
		Data   any    `json:"data"`
	}
	if err := json.Unmarshal(paramsBytes, &logParams); err != nil {
		return nil
	}
	// Convert Data to string
	var dataStr string
	switch v := logParams.Data.(type) {
	case string:
		dataStr = v
	default:
		dataBytes, _ := json.Marshal(v)
		dataStr = string(dataBytes)
	}
	return &LogNotification{
		Level:  logParams.Level,
		Logger: logParams.Logger,
		Data:   dataStr,
	}
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
