package mcplog

import (
	"context"
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// classifyK8sError maps a Kubernetes API error to a log level and message.
// Returns the level, message, and true if the error should be logged.
// Returns zero values and false for nil errors or non-Kubernetes errors.
func classifyK8sError(err error, operation string) (Level, string, bool) {
	if err == nil {
		return 0, "", false
	}

	if apierrors.IsNotFound(err) {
		return LevelInfo, "Resource not found - it may not exist or may have been deleted", true
	} else if apierrors.IsForbidden(err) {
		return LevelError, "Permission denied - check RBAC permissions for " + operation, true
	} else if apierrors.IsUnauthorized(err) {
		return LevelError, "Authentication failed - check cluster credentials", true
	} else if apierrors.IsAlreadyExists(err) {
		return LevelWarning, "Resource already exists", true
	} else if apierrors.IsInvalid(err) {
		return LevelError, "Invalid resource specification - check resource definition", true
	} else if apierrors.IsBadRequest(err) {
		return LevelError, "Invalid request - check parameters", true
	} else if apierrors.IsConflict(err) {
		return LevelError, "Resource conflict - resource may have been modified", true
	} else if apierrors.IsTimeout(err) {
		return LevelError, "Request timeout - cluster may be slow or overloaded", true
	} else if apierrors.IsServerTimeout(err) {
		return LevelError, "Server timeout - cluster may be slow or overloaded", true
	} else if apierrors.IsServiceUnavailable(err) {
		return LevelError, "Service unavailable - cluster may be unreachable", true
	} else if apierrors.IsTooManyRequests(err) {
		return LevelWarning, "Rate limited - too many requests to the cluster", true
	} else {
		var apiStatus apierrors.APIStatus
		if errors.As(err, &apiStatus) {
			return LevelError, "Operation failed - cluster may be unreachable or experiencing issues", true
		}
	}
	return 0, "", false
}

// HandleK8sError sends appropriate MCP log messages based on Kubernetes API error types.
// operation should describe the operation (e.g., "pod access", "deployment deletion").
func HandleK8sError(ctx context.Context, err error, operation string) {
	if level, message, ok := classifyK8sError(err, operation); ok {
		SendMCPLog(ctx, level, message)
	}
}
