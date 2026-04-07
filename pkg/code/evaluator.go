package code

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dop251/goja"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

//go:embed k8s_proxy.js
var k8sProxyScript string

// safeContext wraps a context.Context to expose only cancellation functionality.
// This prevents JavaScript code from accessing sensitive values that may be stored
// in the context (like authentication tokens, user credentials, or request metadata).
type safeContext struct {
	ctx context.Context
}

// Deadline returns the time when work done on behalf of this context should be canceled.
func (s *safeContext) Deadline() (deadline time.Time, ok bool) {
	return s.ctx.Deadline()
}

// Done returns a channel that's closed when work done on behalf of this context should be canceled.
func (s *safeContext) Done() <-chan struct{} {
	return s.ctx.Done()
}

// Err returns nil if Done is not yet closed, or a non-nil error explaining why.
func (s *safeContext) Err() error {
	return s.ctx.Err()
}

// Value always returns nil to prevent access to context values from JavaScript.
// This is intentional for security - context values may contain sensitive data.
func (s *safeContext) Value(_ any) any {
	return nil
}

// newSafeContext creates a context wrapper that only exposes cancellation functionality.
func newSafeContext(ctx context.Context) context.Context {
	return &safeContext{ctx: ctx}
}

// Resource limits for script execution.
//
// Note on memory limits: Goja does not provide built-in memory limiting.
// Memory usage is bounded indirectly by:
//   - MaxCallStackSize preventing deep recursion
//   - Timeout preventing long-running allocations
//   - No file/network I/O limiting external data ingestion
//
// For production deployments with untrusted scripts, consider running
// the MCP server in a container with memory limits (e.g., --memory flag).
const (
	// DefaultTimeout is the default execution timeout for scripts
	DefaultTimeout = 30 * time.Second
	// MaxTimeout is the maximum allowed execution timeout
	MaxTimeout = 5 * time.Minute
	// MaxCallStackSize limits function call depth to prevent stack overflow from infinite recursion.
	// This is a defense against DoS attacks using deeply recursive code.
	MaxCallStackSize = 1024
)

// Evaluator executes JavaScript code with access to Kubernetes clients.
// The evaluator is configured once and can be used for a single evaluation.
type Evaluator struct {
	vm        *goja.Runtime
	namespace string
}

// NewEvaluator creates a new JavaScript evaluator with access to the provided Kubernetes client.
// The kubernetes client is exposed as 'k8s' global, the context as 'ctx', and the default
// namespace as 'namespace'.
func NewEvaluator(ctx context.Context, kubernetes api.KubernetesClient) (*Evaluator, error) {
	vm := goja.New()
	namespace := kubernetes.NamespaceOrDefault("")

	vm.SetMaxCallStackSize(MaxCallStackSize)

	// Configure field name mapping to use JSON struct tags.
	// This allows JavaScript code to access Kubernetes objects using standard JSON field names
	// (e.g., pod.metadata.name, pod.spec.containers) instead of Go field names
	// (e.g., pod.ObjectMeta.Name, pod.Spec.Containers).
	vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))

	// Set up Kubernetes client proxy for transparent resource handling.
	// The proxy script (k8s_proxy.js) sets up transparent conversion of standard Kubernetes
	// YAML/JSON structure to the format expected by Go's Kubernetes client structs.
	// It uses JavaScript Proxy on empty objects (not native Go objects) to avoid
	// invariant violations while still intercepting method calls and converting
	// objects with 'metadata' fields to the flattened Go struct format.
	if _, err := vm.RunString(k8sProxyScript); err != nil {
		return nil, fmt.Errorf("failed to set up k8s proxy: %w", err)
	}

	// Set up __goToJSON helper function for converting Go objects to pure JavaScript objects.
	// This is used internally by the k8s proxy (__toJS function) to convert Go objects
	// with custom types (like Quantity) that don't serialize correctly with JavaScript's
	// JSON.stringify. This helper uses Go's json.Marshal which handles MarshalJSON
	// implementations correctly. It is intentionally not exposed to user scripts.
	if err := vm.Set("__goToJSON", func(obj interface{}) (string, error) {
		jsonBytes, err := json.Marshal(obj)
		if err != nil {
			return "", fmt.Errorf("JSON marshal failed: %w", err)
		}
		return string(jsonBytes), nil
	}); err != nil {
		return nil, fmt.Errorf("failed to set __goToJSON helper: %w", err)
	}

	// Set up __gvr helper function for creating GroupVersionResource objects.
	// This is needed because schema.GroupVersionResource doesn't have json tags,
	// so the TagFieldNameMapper cannot convert JavaScript objects to it.
	// The proxy uses this to properly pass GVR to dynamicClient().resource().
	if err := vm.Set("__gvr", func(group, version, resource string) schema.GroupVersionResource {
		return schema.GroupVersionResource{
			Group:    group,
			Version:  version,
			Resource: resource,
		}
	}); err != nil {
		return nil, fmt.Errorf("failed to set __gvr helper: %w", err)
	}

	// Set up __toUnstructured helper for converting JS objects to *unstructured.Unstructured.
	// This is needed for the dynamic client which expects *unstructured.Unstructured objects.
	// We use JSON marshaling to properly handle the conversion since it preserves the
	// nested structure (metadata.name, etc.) that the dynamic client expects.
	if err := vm.Set("__toUnstructured", func(obj interface{}) (*unstructured.Unstructured, error) {
		// Convert to JSON and back to get a clean map[string]interface{}
		jsonBytes, err := json.Marshal(obj)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal object: %w", err)
		}
		var data map[string]interface{}
		if err := json.Unmarshal(jsonBytes, &data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal object: %w", err)
		}
		return &unstructured.Unstructured{Object: data}, nil
	}); err != nil {
		return nil, fmt.Errorf("failed to set __toUnstructured helper: %w", err)
	}

	// Set up SDK globals once during construction
	// First set the raw client, then wrap it with the proxy for auto-conversion
	if err := vm.Set("__rawK8s", kubernetes); err != nil {
		return nil, fmt.Errorf("failed to set k8s client: %w", err)
	}
	if _, err := vm.RunString("const k8s = __wrapK8sClient(__rawK8s);"); err != nil {
		return nil, fmt.Errorf("failed to wrap k8s client: %w", err)
	}
	// Wrap context to expose only cancellation functionality, not context values.
	// Context values may contain sensitive data (auth tokens, user info, etc.)
	// that should not be accessible from JavaScript.
	if err := vm.Set("ctx", newSafeContext(ctx)); err != nil {
		return nil, fmt.Errorf("failed to set context: %w", err)
	}
	if err := vm.Set("namespace", namespace); err != nil {
		return nil, fmt.Errorf("failed to set namespace: %w", err)
	}

	return &Evaluator{
		vm:        vm,
		namespace: namespace,
	}, nil
}

// Evaluate executes the provided JavaScript script and returns the result.
// The script has access to:
//   - k8s: The Kubernetes client for API operations
//   - ctx: The request context
//   - namespace: The default namespace
//
// The timeout parameter controls how long the script is allowed to run.
// If timeout is <= 0, DefaultTimeout is used.
// If timeout > MaxTimeout, MaxTimeout is used.
func (e *Evaluator) Evaluate(script string, timeout time.Duration) (string, error) {
	if script == "" {
		return "", fmt.Errorf("script cannot be empty")
	}

	// Normalize timeout
	timeout = normalizeTimeout(timeout)

	// Set up timeout interruption
	timer := time.AfterFunc(timeout, func() {
		e.vm.Interrupt("execution timeout exceeded")
	})
	defer timer.Stop()

	// Execute the script
	value, err := e.vm.RunString(script)
	if err != nil {
		return "", wrapError(err)
	}

	// Convert result to string
	return valueToString(value)
}

// wrapError converts Goja errors into user-friendly errors.
// Error messages are sanitized to avoid exposing internal implementation details
// while still providing useful debugging information for script authors.
func wrapError(err error) error {
	// Check if it was an interrupt (timeout)
	var interrupted *goja.InterruptedError
	if errors.As(err, &interrupted) {
		return fmt.Errorf("script interrupted: %v", interrupted.Value())
	}
	// Check if it was a JavaScript exception
	var exception *goja.Exception
	if errors.As(err, &exception) {
		// Extract just the error message without full stack trace
		// The exception.Value() contains the error object, exception.String() includes stack
		if exception.Value() != nil {
			return fmt.Errorf("script error: %s", exception.Value().String())
		}
		return fmt.Errorf("script error: %s", exception.Error())
	}
	// For other errors, provide a generic message without exposing internal details
	return fmt.Errorf("script execution failed: %v", err)
}

// normalizeTimeout ensures the timeout is within valid bounds.
func normalizeTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return DefaultTimeout
	}
	if timeout > MaxTimeout {
		return MaxTimeout
	}
	return timeout
}

// valueToString converts a Goja value to a string representation.
func valueToString(value goja.Value) (string, error) {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return "", nil
	}

	// Export to Go value
	exported := value.Export()

	// Handle different types
	switch v := exported.(type) {
	case string:
		return v, nil
	case bool, int, int64, float64:
		return fmt.Sprintf("%v", v), nil
	default:
		// For complex types, marshal to compact JSON (no indentation to minimize token usage)
		jsonBytes, err := json.Marshal(exported)
		if err != nil {
			return fmt.Sprintf("%v", exported), nil
		}
		return string(jsonBytes), nil
	}
}
