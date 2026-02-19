package api

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// HTTPValidationRequest contains info extracted from an HTTP request for validation.
type HTTPValidationRequest struct {
	GVR          *schema.GroupVersionResource
	GVK          *schema.GroupVersionKind
	HTTPMethod   string // GET, POST, PUT, DELETE, PATCH
	Verb         string // get, list, create, update, delete, patch
	Namespace    string
	ResourceName string
	Body         []byte // For create/update validation
	Path         string
}

// HTTPValidator validates HTTP requests before they reach the K8s API server.
type HTTPValidator interface {
	Validate(ctx context.Context, req *HTTPValidationRequest) error
	Name() string
}

// ValidationErrorCode categorizes validation failures.
type ValidationErrorCode string

const (
	ErrorCodeResourceNotFound ValidationErrorCode = "RESOURCE_NOT_FOUND"
	ErrorCodeInvalidField     ValidationErrorCode = "INVALID_FIELD"
	ErrorCodePermissionDenied ValidationErrorCode = "PERMISSION_DENIED"
)

// ValidationError provides AI-friendly error information for validation failures.
type ValidationError struct {
	Code    ValidationErrorCode
	Message string
	Field   string // optional, for field-level errors
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Validation Error [%s]: %s", e.Code, e.Message))

	if e.Field != "" {
		sb.WriteString(fmt.Sprintf("\n  Field: %s", e.Field))
	}

	return sb.String()
}

// NewPermissionDeniedError creates an error for RBAC permission failures.
func NewPermissionDeniedError(verb, resource, namespace string) *ValidationError {
	var msg string
	if namespace != "" {
		msg = fmt.Sprintf("Cannot %s %s in namespace %q", verb, resource, namespace)
	} else {
		msg = fmt.Sprintf("Cannot %s %s (cluster-scoped)", verb, resource)
	}

	return &ValidationError{
		Code:    ErrorCodePermissionDenied,
		Message: msg,
	}
}

// FormatResourceName creates a human-readable resource identifier from GVR.
func FormatResourceName(gvr *schema.GroupVersionResource) string {
	if gvr == nil {
		return "unknown"
	}
	if gvr.Group == "" {
		return gvr.Resource
	}
	return gvr.Resource + "." + gvr.Group
}
