// Package dynamic exposes MCP tools that are defined declaratively as Tool
// custom resources (tools.kubernetes-mcp-server.io/v1alpha1) installed in the
// cluster, instead of being compiled into the server.
//
// A Tool resource declares the MCP-facing contract (name, description, input
// schema, annotations) and exactly one action that the server performs on
// behalf of the caller with the caller's own identity:
//
//   - kubernetes: a single request against the Kubernetes API (any verb,
//     any GVK, optional subresource). This is the "thin passthrough with
//     guardrails" every built-in tool is ultimately made of.
//   - http: an HTTP request to an arbitrary URL or to an in-cluster Service
//     (through the API server's service proxy), authenticated with the
//     caller's bearer token. This is what the Kiali and NetObserv toolsets do.
//
// An optional CEL expression (spec.output) projects the raw response into
// what the model should see.
package dynamic

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// Group is the API group of the Tool custom resource.
	Group = "tools.kubernetes-mcp-server.io"
	// Version is the API version of the Tool custom resource.
	Version = "v1alpha1"
	// Kind is the kind of the Tool custom resource.
	Kind = "Tool"
	// Resource is the plural resource name of the Tool custom resource.
	Resource = "tools"
)

// GVR identifies the Tool custom resource.
var GVR = schema.GroupVersionResource{Group: Group, Version: Version, Resource: Resource}

var (
	validToolName = regexp.MustCompile(`^[a-z0-9][a-z0-9_]*$`)
	validVerbs    = map[string]struct{}{
		"get": {}, "list": {}, "create": {}, "update": {}, "patch": {}, "delete": {},
	}
)

// Tool mirrors the Tool custom resource.
type Tool struct {
	Metadata Metadata `json:"metadata"`
	Spec     ToolSpec `json:"spec"`
}

// Metadata is the subset of ObjectMeta the toolset needs.
type Metadata struct {
	Name            string `json:"name"`
	ResourceVersion string `json:"resourceVersion,omitempty"`
}

// ToolSpec is the declarative definition of an MCP tool.
type ToolSpec struct {
	// Name is the MCP tool name. Defaults to metadata.name with '-' and '.'
	// replaced by '_'.
	Name string `json:"name,omitempty"`
	// Title is the human-readable tool title (annotation).
	Title string `json:"title,omitempty"`
	// Description is written for the model, not for humans.
	Description string `json:"description"`
	// ReadOnly, Destructive and Idempotent map to the MCP tool annotations and
	// drive the server's read_only / disable_destructive filtering.
	ReadOnly    bool `json:"readOnly,omitempty"`
	Destructive bool `json:"destructive,omitempty"`
	Idempotent  bool `json:"idempotent,omitempty"`
	// InputSchema is a JSON Schema object describing the tool arguments.
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
	// Exactly one of Kubernetes, HTTP or Steps must be set.
	Kubernetes *KubernetesAction `json:"kubernetes,omitempty"`
	HTTP       *HTTPAction       `json:"http,omitempty"`
	// Steps composes several actions. Each step sees the results of the
	// previous ones as `steps.<name>` in every expression.
	Steps []Step `json:"steps,omitempty"`
	// Output is an optional CEL expression evaluated with the variables
	// `response` (decoded action response) and `args` (tool arguments). Its
	// result replaces the raw response in the tool result.
	Output string `json:"output,omitempty"`
}

// Step is one action of a composed Tool. Exactly one of Kubernetes, HTTP or
// Expression must be set.
type Step struct {
	// Name is how later expressions address the result: steps.<name>.
	Name       string            `json:"name"`
	Kubernetes *KubernetesAction `json:"kubernetes,omitempty"`
	HTTP       *HTTPAction       `json:"http,omitempty"`
	// Expression is a pure CEL step: no request, its value is the result.
	Expression string `json:"expression,omitempty"`
	// When is a CEL boolean; when false the step is skipped and its result
	// is null.
	When string `json:"when,omitempty"`
	// ForEach is a CEL expression producing a list. The action runs once per
	// element with `item` bound to it, and the step result is the list of
	// results in the same order.
	ForEach string `json:"forEach,omitempty"`
	// IgnoreError records a failed request as {"error": message} instead of
	// failing the whole tool.
	IgnoreError bool `json:"ignoreError,omitempty"`
}

const (
	// MaxSteps bounds the number of steps of one Tool.
	MaxSteps = 16
	// MaxForEach bounds the fan-out of one forEach step.
	MaxForEach = 64
)

var validStepName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// Ref resolves a string from a tool argument, a CEL expression over `args`,
// or a fixed value, in that order of precedence (the first non-empty wins).
// A fixed value may contain `{argument}` placeholders. In YAML a plain string
// is shorthand for {value: string}.
type Ref struct {
	Argument   string `json:"argument,omitempty"`
	Expression string `json:"expression,omitempty"`
	Value      string `json:"value,omitempty"`
}

// UnmarshalJSON accepts either a plain string (a fixed value) or an object.
func (r *Ref) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		return json.Unmarshal(data, &r.Value)
	}
	type plain Ref
	return json.Unmarshal(data, (*plain)(r))
}

// MarshalJSON writes a value-only Ref as a plain string.
func (r Ref) MarshalJSON() ([]byte, error) {
	if r.Argument == "" && r.Expression == "" {
		return json.Marshal(r.Value)
	}
	type plain Ref
	return json.Marshal(plain(r))
}

// NamespaceRef is a Ref that can additionally select all namespaces.
// Resolution order: the argument, the expression, the fixed value, all
// namespaces (when All is set), otherwise the caller's default namespace.
type NamespaceRef struct {
	Argument   string `json:"argument,omitempty"`
	Expression string `json:"expression,omitempty"`
	Value      string `json:"value,omitempty"`
	All        bool   `json:"all,omitempty"`
}

func (n *NamespaceRef) ref() *Ref {
	if n == nil {
		return nil
	}
	return &Ref{Argument: n.Argument, Expression: n.Expression, Value: n.Value}
}

// Body describes where a request body comes from: a tool argument (a YAML or
// JSON string, or an object), a fixed JSON value, or a CEL expression over
// `args`.
type Body struct {
	Argument   string          `json:"argument,omitempty"`
	Value      json.RawMessage `json:"value,omitempty"`
	Expression string          `json:"expression,omitempty"`
}

// KubernetesAction performs one request against the Kubernetes API.
type KubernetesAction struct {
	// Verb is one of get, list, create, update, patch, delete. It maps to the
	// HTTP method and is also what the RBAC metadata advertises.
	Verb       string `json:"verb"`
	APIVersion string `json:"apiVersion"`
	// Kind is resolved to a resource through the target's RESTMapper.
	// Alternatively set Resource (the plural) to skip discovery, which is
	// needed for aggregated APIs that only serve subresources (for example
	// subresources.kubevirt.io).
	Kind     string `json:"kind,omitempty"`
	Resource string `json:"resource,omitempty"`
	// Namespaced only applies with Resource and defaults to true.
	Namespaced *bool `json:"namespaced,omitempty"`
	// Subresource may contain several segments (`proxy/stats/summary`),
	// `{argument}` placeholders, or be a full Ref ({argument}/{expression}).
	Subresource *Ref          `json:"subresource,omitempty"`
	Namespace   *NamespaceRef `json:"namespace,omitempty"`
	Name        *Ref          `json:"name,omitempty"`
	// Query maps query parameters to references; QueryExpression is a CEL
	// expression over `args` producing a map of additional parameters.
	// References win over the expression.
	Query           map[string]Ref `json:"query,omitempty"`
	QueryExpression string         `json:"queryExpression,omitempty"`
	Body            *Body          `json:"body,omitempty"`
}

// ServiceRef addresses an in-cluster Service reached through the API server's
// service proxy (/api/v1/namespaces/{ns}/services/{scheme}:{name}:{port}/proxy).
type ServiceRef struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Port      string `json:"port,omitempty"`
	Scheme    string `json:"scheme,omitempty"`
}

// HTTPAction performs one HTTP request authenticated with the caller's
// bearer token. Exactly one of URL or Service must be set.
type HTTPAction struct {
	Method  string      `json:"method,omitempty"`
	URL     string      `json:"url,omitempty"`
	Service *ServiceRef `json:"service,omitempty"`
	// Path is appended to the base URL. `{argument}` placeholders are replaced
	// with the URL-escaped tool argument of that name.
	Path            string            `json:"path,omitempty"`
	Query           map[string]Ref    `json:"query,omitempty"`
	QueryExpression string            `json:"queryExpression,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	Body            *Body             `json:"body,omitempty"`
	// CertificateAuthority is a PEM bundle used to verify the URL's server.
	CertificateAuthority  string `json:"certificateAuthority,omitempty"`
	InsecureSkipTLSVerify bool   `json:"insecureSkipTLSVerify,omitempty"`
}

// ToolName returns the MCP tool name for the resource.
func (t *Tool) ToolName() string {
	if t.Spec.Name != "" {
		return t.Spec.Name
	}
	return strings.NewReplacer("-", "_", ".", "_").Replace(t.Metadata.Name)
}

// Validate checks the static parts of the definition.
func (t *Tool) Validate() error {
	var errs []error
	if !validToolName.MatchString(t.ToolName()) {
		errs = append(errs, fmt.Errorf("tool name %q must match %s", t.ToolName(), validToolName))
	}
	if strings.TrimSpace(t.Spec.Description) == "" {
		errs = append(errs, errors.New("spec.description is required"))
	}
	set := 0
	for _, present := range []bool{t.Spec.Kubernetes != nil, t.Spec.HTTP != nil, len(t.Spec.Steps) > 0} {
		if present {
			set++
		}
	}
	switch {
	case set > 1:
		errs = append(errs, errors.New("spec.kubernetes, spec.http and spec.steps are mutually exclusive"))
	case t.Spec.Kubernetes != nil:
		errs = append(errs, t.Spec.Kubernetes.validate()...)
	case t.Spec.HTTP != nil:
		errs = append(errs, t.Spec.HTTP.validate()...)
	case len(t.Spec.Steps) > 0:
		errs = append(errs, validateSteps(t.Spec.Steps)...)
	default:
		errs = append(errs, errors.New("one of spec.kubernetes, spec.http or spec.steps is required"))
	}
	return errors.Join(errs...)
}

func validateSteps(steps []Step) []error {
	var errs []error
	if len(steps) > MaxSteps {
		errs = append(errs, fmt.Errorf("spec.steps has %d steps, at most %d are allowed", len(steps), MaxSteps))
	}
	seen := map[string]bool{}
	for i, step := range steps {
		path := fmt.Sprintf("spec.steps[%d]", i)
		if !validStepName.MatchString(step.Name) {
			errs = append(errs, fmt.Errorf("%s.name %q must match %s", path, step.Name, validStepName))
		}
		if seen[step.Name] {
			errs = append(errs, fmt.Errorf("%s.name %q is duplicated", path, step.Name))
		}
		seen[step.Name] = true
		set := 0
		for _, present := range []bool{step.Kubernetes != nil, step.HTTP != nil, step.Expression != ""} {
			if present {
				set++
			}
		}
		switch {
		case set != 1:
			errs = append(errs, fmt.Errorf("%s requires exactly one of kubernetes, http or expression", path))
		case step.Kubernetes != nil:
			errs = append(errs, step.Kubernetes.validate()...)
		case step.HTTP != nil:
			errs = append(errs, step.HTTP.validate()...)
		}
	}
	return errs
}

func (a *KubernetesAction) validate() []error {
	var errs []error
	if _, ok := validVerbs[a.Verb]; !ok {
		errs = append(errs, fmt.Errorf("spec.kubernetes.verb %q must be one of get, list, create, update, patch, delete", a.Verb))
	}
	if a.APIVersion == "" || (a.Kind == "") == (a.Resource == "") {
		errs = append(errs, errors.New("spec.kubernetes.apiVersion and exactly one of spec.kubernetes.kind or spec.kubernetes.resource are required"))
	}
	if a.Body != nil {
		errs = append(errs, a.Body.validate("spec.kubernetes.body")...)
	}
	return errs
}

func (a *HTTPAction) validate() []error {
	var errs []error
	if (a.URL == "") == (a.Service == nil) {
		errs = append(errs, errors.New("exactly one of spec.http.url or spec.http.service is required"))
	}
	if a.URL != "" && !strings.HasPrefix(a.URL, "http://") && !strings.HasPrefix(a.URL, "https://") {
		errs = append(errs, fmt.Errorf("spec.http.url %q must be an absolute http(s) URL", a.URL))
	}
	if a.Service != nil && (a.Service.Namespace == "" || a.Service.Name == "") {
		errs = append(errs, errors.New("spec.http.service.namespace and spec.http.service.name are required"))
	}
	if a.Body != nil {
		errs = append(errs, a.Body.validate("spec.http.body")...)
	}
	return errs
}

func (b *Body) validate(path string) []error {
	set := 0
	if b.Argument != "" {
		set++
	}
	if len(b.Value) > 0 {
		set++
	}
	if b.Expression != "" {
		set++
	}
	if set != 1 {
		return []error{fmt.Errorf("%s requires exactly one of argument, value or expression", path)}
	}
	return nil
}

// refs returns every Ref of the spec, so their expressions can be compiled
// once at registration time.
func (t *ToolSpec) refs() []*Ref {
	var refs []*Ref
	refs = append(refs, t.Kubernetes.refs()...)
	refs = append(refs, t.HTTP.refs()...)
	for _, step := range t.Steps {
		refs = append(refs, step.Kubernetes.refs()...)
		refs = append(refs, step.HTTP.refs()...)
	}
	return refs
}

func (a *KubernetesAction) refs() []*Ref {
	if a == nil {
		return nil
	}
	refs := []*Ref{a.Namespace.ref(), a.Name, a.Subresource}
	for key := range a.Query {
		ref := a.Query[key]
		refs = append(refs, &ref)
	}
	return refs
}

func (a *HTTPAction) refs() []*Ref {
	if a == nil {
		return nil
	}
	var refs []*Ref
	for key := range a.Query {
		ref := a.Query[key]
		refs = append(refs, &ref)
	}
	return refs
}
