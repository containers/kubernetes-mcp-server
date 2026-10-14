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
	// Group is the API group of the Tool and Prompt custom resources.
	Group = "tools.kubernetes-mcp-server.io"
	// Version is the API version of the Tool and Prompt custom resources.
	Version = "v1alpha1"
	// Kind is the kind of the Tool custom resource.
	Kind = "Tool"
	// Resource is the plural resource name of the Tool custom resource.
	Resource = "tools"
	// PromptKind is the kind of the Prompt custom resource.
	PromptKind = "Prompt"
	// PromptResource is the plural resource name of the Prompt custom resource.
	PromptResource = "prompts"

	// ResponseJSON decodes the response body as JSON (falling back to text).
	ResponseJSON = "json"
	// ResponseText keeps the response body as text.
	ResponseText = "text"
	// ResponseBinary keeps the response body as bytes (e.g. an image).
	ResponseBinary = "binary"
)

// GVR identifies the Tool custom resource.
var GVR = schema.GroupVersionResource{Group: Group, Version: Version, Resource: Resource}

// PromptGVR identifies the Prompt custom resource.
var PromptGVR = schema.GroupVersionResource{Group: Group, Version: Version, Resource: PromptResource}

// GVKRef names an API kind a Tool depends on.
type GVKRef struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

// Prompt mirrors the Prompt custom resource.
type Prompt struct {
	Metadata Metadata   `json:"metadata"`
	Spec     PromptSpec `json:"spec"`
}

// PromptSpec is the declarative definition of an MCP prompt: optional
// gathering steps, then messages rendered from text templates.
type PromptSpec struct {
	// Name is the MCP prompt name. Defaults to metadata.name.
	Name        string           `json:"name,omitempty"`
	Title       string           `json:"title,omitempty"`
	Description string           `json:"description"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
	Steps       []Step           `json:"steps,omitempty"`
	Messages    []PromptMessage  `json:"messages"`
}

// PromptArgument mirrors the MCP prompt argument.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// PromptMessage is one message of the rendered prompt. Text is a template
// with `${expression}` placeholders evaluated over args and steps; When is an
// optional CEL boolean that drops the message when false.
type PromptMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
	When string `json:"when,omitempty"`
}

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
	// Requires lists API kinds the tool depends on. When target compatibility
	// filters are enabled, the tool is hidden if no target serves them.
	Requires []GVKRef `json:"requires,omitempty"`
}

// Step is one action of a composed Tool. Exactly one of Kubernetes, HTTP,
// Expression, Fail or Elicit must be set.
type Step struct {
	// Name is how later expressions address the result: steps.<name>.
	Name       string            `json:"name"`
	Kubernetes *KubernetesAction `json:"kubernetes,omitempty"`
	HTTP       *HTTPAction       `json:"http,omitempty"`
	// Expression is a pure CEL step: no request, its value is the result.
	Expression string `json:"expression,omitempty"`
	// Fail is a CEL string; when it evaluates to a non-empty string the tool
	// fails with that message. Use it for argument and state checks.
	Fail string `json:"fail,omitempty"`
	// Elicit asks the user a question through MCP elicitation. The result is
	// {"action": "accept"|"decline"|"cancel"|"unsupported", "content": {...}}.
	Elicit *ElicitStep `json:"elicit,omitempty"`
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

// ElicitStep asks the user for confirmation or input. Message is a text
// template with `${expression}` placeholders.
type ElicitStep struct {
	Message string `json:"message"`
}

const (
	// MaxSteps bounds the number of steps of one Tool.
	MaxSteps = 16
	// MaxRange bounds the list produced by the range(n) CEL function.
	MaxRange = 10_000
	// CELCostLimit bounds the runtime cost of one expression evaluation (the
	// same per-call limit the Kubernetes API server applies to CEL rules).
	CELCostLimit = 1_000_000
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
	// Response is json (default), text or binary. MIMEType labels a binary
	// response (default image/png when the server does not say).
	Response string `json:"response,omitempty"`
	MIMEType string `json:"mimeType,omitempty"`
	// Redact removes secrets from a text response (see pkg/redact).
	Redact bool `json:"redact,omitempty"`
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
	// Response is json (default), text or binary; MIMEType overrides the
	// Content-Type of a binary response. Redact removes secrets from text.
	Response string `json:"response,omitempty"`
	MIMEType string `json:"mimeType,omitempty"`
	Redact   bool   `json:"redact,omitempty"`
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
	for i, req := range t.Spec.Requires {
		if req.APIVersion == "" || req.Kind == "" {
			errs = append(errs, fmt.Errorf("spec.requires[%d] needs apiVersion and kind", i))
		}
	}
	return errors.Join(errs...)
}

// PromptName returns the MCP prompt name for the resource.
func (p *Prompt) PromptName() string {
	if p.Spec.Name != "" {
		return p.Spec.Name
	}
	return p.Metadata.Name
}

// Validate checks the static parts of a Prompt definition.
func (p *Prompt) Validate() error {
	var errs []error
	if strings.TrimSpace(p.PromptName()) == "" {
		errs = append(errs, errors.New("a prompt name is required"))
	}
	if strings.TrimSpace(p.Spec.Description) == "" {
		errs = append(errs, errors.New("spec.description is required"))
	}
	if len(p.Spec.Messages) == 0 {
		errs = append(errs, errors.New("spec.messages must have at least one message"))
	}
	for i, message := range p.Spec.Messages {
		if message.Role != "user" && message.Role != "assistant" {
			errs = append(errs, fmt.Errorf("spec.messages[%d].role %q must be user or assistant", i, message.Role))
		}
	}
	seen := map[string]bool{}
	for i, argument := range p.Spec.Arguments {
		if argument.Name == "" {
			errs = append(errs, fmt.Errorf("spec.arguments[%d].name is required", i))
		}
		if seen[argument.Name] {
			errs = append(errs, fmt.Errorf("spec.arguments[%d].name %q is duplicated", i, argument.Name))
		}
		seen[argument.Name] = true
	}
	errs = append(errs, validateSteps(p.Spec.Steps)...)
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
		for _, present := range []bool{step.Kubernetes != nil, step.HTTP != nil, step.Expression != "", step.Fail != "", step.Elicit != nil} {
			if present {
				set++
			}
		}
		switch {
		case set != 1:
			errs = append(errs, fmt.Errorf("%s requires exactly one of kubernetes, http, expression, fail or elicit", path))
		case step.Elicit != nil && strings.TrimSpace(step.Elicit.Message) == "":
			errs = append(errs, fmt.Errorf("%s.elicit.message is required", path))
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
	errs = append(errs, validateResponse("spec.kubernetes", a.Response)...)
	return errs
}

func validateResponse(path, response string) []error {
	switch response {
	case "", ResponseJSON, ResponseText, ResponseBinary:
		return nil
	}
	return []error{fmt.Errorf("%s.response %q must be one of json, text, binary", path, response)}
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
	errs = append(errs, validateResponse("spec.http", a.Response)...)
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
