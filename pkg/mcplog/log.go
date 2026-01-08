package mcplog

import (
	"context"
	"regexp"

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

var (
	// mcpLogger is a dedicated named logger for MCP client-facing logs
	// This provides complete separation from server logs
	// issue for the sdk to implement this https://github.com/modelcontextprotocol/go-sdk/issues/748
	mcpLogger logr.Logger = klog.NewKlogr().WithName("mcp")

	sensitivePatterns = []*regexp.Regexp{
		// Generic JSON/YAML fields
		regexp.MustCompile(`("password"\s*:\s*)"[^"]*"`),
		regexp.MustCompile(`("token"\s*:\s*)"[^"]*"`),
		regexp.MustCompile(`("secret"\s*:\s*)"[^"]*"`),
		regexp.MustCompile(`("api[_-]?key"\s*:\s*)"[^"]*"`),
		regexp.MustCompile(`("access[_-]?key"\s*:\s*)"[^"]*"`),
		regexp.MustCompile(`("client[_-]?secret"\s*:\s*)"[^"]*"`),
		regexp.MustCompile(`("private[_-]?key"\s*:\s*)"[^"]*"`),
		// Authorization headers
		regexp.MustCompile(`(Bearer\s+)[A-Za-z0-9\-._~+/]+=*`),
		regexp.MustCompile(`(Basic\s+)[A-Za-z0-9+/]+=*`),
		// AWS credentials
		regexp.MustCompile(`(AKIA[0-9A-Z]{16})`),
		regexp.MustCompile(`(aws_secret_access_key\s*=\s*)([A-Za-z0-9/+=]{40})`),
		regexp.MustCompile(`(A3T[A-Z0-9]|AKIA|AGPA|AIDA|AROA|AIPA|ANPA|ANVA|ASIA)[A-Z0-9]{16}`),
		// GitHub tokens
		regexp.MustCompile(`(ghp_[a-zA-Z0-9]{36})`),
		regexp.MustCompile(`(github_pat_[a-zA-Z0-9]{22}_[a-zA-Z0-9]{59})`),
		// GitLab tokens
		regexp.MustCompile(`(glpat-[a-zA-Z0-9\-_]{20})`),
		// GCP
		regexp.MustCompile(`(AIza[0-9A-Za-z\-_]{35})`),
		// Azure
		regexp.MustCompile(`(AccountKey=[A-Za-z0-9+/]{88}==)`),
		// OpenAI / Anthropic
		regexp.MustCompile(`(sk-proj-[a-zA-Z0-9]{48})`),
		regexp.MustCompile(`(sk-ant-api03-[a-zA-Z0-9\-_]{95})`),
		// JWT tokens
		regexp.MustCompile(`(eyJ[a-zA-Z0-9_-]+\.eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+)`),
		// Private keys
		regexp.MustCompile(`(-----BEGIN[A-Z ]+PRIVATE KEY-----)`),
		regexp.MustCompile(`(-----BEGIN RSA PRIVATE KEY-----)`),
		regexp.MustCompile(`(-----BEGIN EC PRIVATE KEY-----)`),
		regexp.MustCompile(`(-----BEGIN OPENSSH PRIVATE KEY-----)`),
		regexp.MustCompile(`(-----BEGIN PGP PRIVATE KEY BLOCK-----)`),
		// Database connection strings
		regexp.MustCompile(`(postgres://[^:]+:)([^@]+)(@)`),
		regexp.MustCompile(`(mysql://[^:]+:)([^@]+)(@)`),
		regexp.MustCompile(`(mongodb(\+srv)?://[^:]+:)([^@]+)(@)`),
	}
)

func sanitizeMessage(msg string) string {
	// JSON/YAML field patterns (indices 0-6) - preserve field name
	for i := 0; i < 7 && i < len(sensitivePatterns); i++ {
		msg = sensitivePatterns[i].ReplaceAllString(msg, `$1"[REDACTED]"`)
	}

	// Authorization headers (indices 7-8) - preserve header type
	for i := 7; i < 9 && i < len(sensitivePatterns); i++ {
		msg = sensitivePatterns[i].ReplaceAllString(msg, `$1[REDACTED]`)
	}

	// Database connection strings (indices 25-27) - preserve URL structure
	if len(sensitivePatterns) > 27 {
		msg = sensitivePatterns[25].ReplaceAllString(msg, `$1[REDACTED]$3`) // PostgreSQL
		msg = sensitivePatterns[26].ReplaceAllString(msg, `$1[REDACTED]$3`) // MySQL
		msg = sensitivePatterns[27].ReplaceAllString(msg, `$1[REDACTED]$4`) // MongoDB
	}

	// All other patterns (AWS, GitHub, tokens, keys, etc.) - redact entire match
	for i := 9; i < len(sensitivePatterns); i++ {
		// Skip database patterns (already handled)
		if i >= 25 && i <= 27 {
			continue
		}
		msg = sensitivePatterns[i].ReplaceAllString(msg, `[REDACTED]`)
	}

	return msg
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

	message = sanitizeMessage(message)

	if err := session.Log(ctx, &mcp.LoggingMessageParams{
		Level:  mcp.LoggingLevel(level.String()),
		Logger: "kubernetes-mcp-server",
		Data:   message,
	}); err != nil {
		mcpLogger.V(3).Info("failed to send log to MCP client", "error", err)
	}
}
