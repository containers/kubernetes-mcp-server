// Package mcplog sends log notifications to MCP clients via the MCP logging capability.
//
// Deprecated: The MCP logging feature is deprecated as of protocol version 2026-07-28 (SEP-2577).
// Migrate to consuming stderr output (for STDIO servers) or OpenTelemetry.
// See https://modelcontextprotocol.io/seps/2577-deprecate-roots-sampling-and-logging.
package mcplog

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/sanitize"
	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/klog/v2"
)

// ContextKey is a type for context keys to avoid collisions
type ContextKey string

// MCPSessionContextKey is the context key for storing MCP ServerSession
const MCPSessionContextKey = ContextKey("mcp_session")

// Level represents MCP log severity levels per RFC 5424 syslog specification.
// https://modelcontextprotocol.io/specification/2025-11-25/server/utilities/logging#log-levels
type Level int

// Log levels from least to most severe, per MCP specification.
const (
	// LevelDebug is for detailed debugging information.
	LevelDebug Level = iota
	// LevelInfo is for general informational messages.
	LevelInfo
	// LevelNotice is for normal but significant events.
	LevelNotice
	// LevelWarning is for warning conditions.
	LevelWarning
	// LevelError is for error conditions.
	LevelError
	// LevelCritical is for critical conditions.
	LevelCritical
	// LevelAlert is for conditions requiring immediate action.
	LevelAlert
	// LevelEmergency is for system unusable conditions.
	LevelEmergency
)

// levelStrings maps Level values to their MCP protocol string representation.
var levelStrings = [...]string{
	LevelDebug:     "debug",
	LevelInfo:      "info",
	LevelNotice:    "notice",
	LevelWarning:   "warning",
	LevelError:     "error",
	LevelCritical:  "critical",
	LevelAlert:     "alert",
	LevelEmergency: "emergency",
}

// String returns the MCP protocol string representation of the level.
func (l Level) String() string {
	if l >= 0 && int(l) < len(levelStrings) {
		return levelStrings[l]
	}
	return "debug"
}

// mcpLogger is a dedicated named logger for MCP client-facing logs.
// This provides complete separation from server logs.
// issue for the sdk to implement this https://github.com/modelcontextprotocol/go-sdk/issues/748
var mcpLogger logr.Logger = klog.NewKlogr().WithName("mcp")

// Sanitize redacts secret-shaped substrings (Bearer/Basic credentials, JWTs,
// JSON "token"/"secret"/"password"/... fields, AWS/GitHub/Anthropic keys,
// private-key PEM blocks, DB connection strings) from msg. It is intended
// for log messages that may inline user-supplied or tool-supplied payloads.
//
// This is best-effort: the patterns are a denylist, and content that doesn't
// match a known shape (e.g. arbitrary YAML kubeconfig fields) passes through
// unchanged. Callers handling structured credentials should still avoid
// logging them in the first place.
func Sanitize(msg string) string {
	return sanitize.Text(msg)
}

// SendMCPLog sends a log notification to the MCP client and server logs.
// Uses dedicated "mcp" named logger. Message is automatically sanitized.
func SendMCPLog(ctx context.Context, level Level, message string) {
	switch level {
	case LevelError, LevelCritical, LevelAlert, LevelEmergency:
		mcpLogger.Error(nil, message)
	case LevelWarning, LevelNotice:
		mcpLogger.V(1).Info(message)
	default:
		mcpLogger.V(2).Info(message)
	}

	session, ok := ctx.Value(MCPSessionContextKey).(*mcp.ServerSession)
	if !ok || session == nil {
		return
	}

	message = Sanitize(message)

	if err := session.Log(ctx, &mcp.LoggingMessageParams{ //nolint:staticcheck // MCP logging deprecated (SEP-2577)
		Level:  mcp.LoggingLevel(level.String()), //nolint:staticcheck // MCP logging deprecated (SEP-2577)
		Logger: "kubernetes-mcp-server",
		Data:   message,
	}); err != nil {
		mcpLogger.V(3).Info("failed to send log to MCP client", "error", err)
	}
}
