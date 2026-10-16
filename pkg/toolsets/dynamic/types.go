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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"regexp"
	"sort"
	"strings"
	"time"

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
	// ToolSetKind is the kind of the ToolSet custom resource.
	ToolSetKind = "ToolSet"
	// ToolSetResource is the plural resource name of the ToolSet custom resource.
	ToolSetResource = "toolsets"
	// IdentityCaller sends the caller's credentials on http actions (default).
	IdentityCaller = "caller"
	// IdentityServer sends the server's own credentials on http actions.
	IdentityServer = "server"

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

// ToolSetGVR identifies the ToolSet custom resource.
var ToolSetGVR = schema.GroupVersionResource{Group: Group, Version: Version, Resource: ToolSetResource}

// ToolSet is the unit of enablement, naming and instructions of a family of
// Tools and Prompts (see docs/specs/tools-for-models.md). Operators enable
// a family with the toolset name "dynamic/<metadata.name>".
type ToolSet struct {
	Metadata Metadata       `json:"metadata"`
	Spec     ToolSetSpec    `json:"spec"`
	Status   ResourceStatus `json:"status,omitempty"`
}

type ToolSetSpec struct {
	// Description is for operators and the toolsets listing.
	Description string `json:"description"`
	// Prefix, when set, is what every tool name of the family must start
	// with (followed by "_"): a model recognizes a family by its prefix.
	Prefix string `json:"prefix,omitempty"`
	// Instructions are contributed to the MCP server instructions under a
	// heading with the ToolSet's name: workflow guidance for the family,
	// said once instead of repeated in every description.
	Instructions string `json:"instructions,omitempty"`
	// Requires is inherited by every Tool of the family (a Tool may add its own).
	Requires []GVKRef `json:"requires,omitempty"`
	// Identity is what http actions of the family send: "caller" (default)
	// or "server" (the server's own credentials, the former auth_mode
	// kubeconfig of the observability toolsets). A family-level decision
	// the RBAC metadata reports.
	Identity string `json:"identity,omitempty"`
}

var validPrefix = regexp.MustCompile(`^[a-z0-9][a-z0-9_]*$`)

// Validate checks the static parts of the definition.
func (t *ToolSet) Validate() error {
	var errs []error
	if t.Metadata.Name == "" {
		errs = append(errs, errors.New("metadata.name is required"))
	}
	if strings.TrimSpace(t.Spec.Description) == "" {
		errs = append(errs, errors.New("spec.description is required"))
	}
	if t.Spec.Prefix != "" && !validPrefix.MatchString(t.Spec.Prefix) {
		errs = append(errs, fmt.Errorf("spec.prefix %q must match %s", t.Spec.Prefix, validPrefix))
	}
	switch t.Spec.Identity {
	case "", IdentityCaller, IdentityServer:
	default:
		errs = append(errs, fmt.Errorf("spec.identity %q must be %s or %s", t.Spec.Identity, IdentityCaller, IdentityServer))
	}
	for i, req := range t.Spec.Requires {
		if req.APIVersion == "" || req.Kind == "" {
			errs = append(errs, fmt.Errorf("spec.requires[%d] needs apiVersion and kind", i))
		}
	}
	return errors.Join(errs...)
}

// ToolsetName is the toolset name operators enable the family with.
func (t *ToolSet) ToolsetName() string {
	return FamilyPrefix + t.Metadata.Name
}

// FamilyPrefix is what the toolset name of a ToolSet family starts with.
const FamilyPrefix = "dynamic/"

// GVKRef names an API kind a Tool depends on.
type GVKRef struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

// Prompt mirrors the Prompt custom resource.
type Prompt struct {
	Metadata Metadata       `json:"metadata"`
	Spec     PromptSpec     `json:"spec"`
	Status   ResourceStatus `json:"status,omitempty"`
}

// PromptSpec is the declarative definition of an MCP prompt: optional
// gathering steps, then messages rendered from text templates.
type PromptSpec struct {
	// ToolSet names the family (a ToolSet resource) the prompt belongs to.
	ToolSet string `json:"toolset,omitempty"`
	// Name is the MCP prompt name. Defaults to metadata.name.
	Name        string           `json:"name,omitempty"`
	Title       string           `json:"title,omitempty"`
	Description string           `json:"description"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
	Steps       []Step           `json:"steps,omitempty"`
	Messages    []PromptMessage  `json:"messages"`
	// Timeout bounds one call of the prompt, as for a Tool.
	Timeout string `json:"timeout,omitempty"`
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
	Metadata Metadata       `json:"metadata"`
	Spec     ToolSpec       `json:"spec"`
	Status   ResourceStatus `json:"status,omitempty"`
}

// Metadata is the subset of ObjectMeta the toolset needs.
type Metadata struct {
	Name            string `json:"name"`
	ResourceVersion string `json:"resourceVersion,omitempty"`
	Generation      int64  `json:"generation,omitempty"`
}

// ResourceStatus is the status the server writes back to a Tool, Prompt or
// ToolSet: one Ready condition saying what the server did with the resource
// and why, so `kubectl get` shows it without reading the server's log.
type ResourceStatus struct {
	// Name is the MCP name the resource is exposed as, when it is.
	Name       string             `json:"name,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ConditionReady is the condition type the server maintains.
const ConditionReady = "Ready"

// ToolSpec is the declarative definition of an MCP tool.
type ToolSpec struct {
	// ToolSet names the family (a ToolSet resource) the tool belongs to:
	// its prefix, instructions, inherited requires and identity.
	ToolSet string `json:"toolset,omitempty"`
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
	// Meta is passed through as the MCP tool's `_meta`, for client-side
	// extensions such as UI renderers keyed by tool.
	Meta map[string]any `json:"meta,omitempty"`
	// Timeout bounds one call of the tool, every step included, as a Go
	// duration ("30s", "2m"). Defaults to DefaultTimeout.
	Timeout string `json:"timeout,omitempty"`
	// OutputFull is the projection behind `detail: full`; declaring it adds
	// the `detail` argument (concise, the `output` projection, is the default).
	OutputFull string `json:"outputFull,omitempty"`
	// Limit bounds a list-shaped result (see docs/specs/tools-for-models.md):
	// the engine adds the `limit` argument, cuts the result to it and says
	// how much there was and how to narrow it. A single kubernetes `list`
	// action is paginated by the API server instead (`continue`).
	Limit *Limit `json:"limit,omitempty"`
	// Variants makes one tool with modes: an argument selects which option
	// runs (start/stop/restart, pipeline/task). The engine adds the argument
	// as a required enum, dispatches, and advertises the union of the
	// options' RBAC. Exactly one of Kubernetes, HTTP, Steps or Variants.
	Variants *Variants `json:"variants,omitempty"`
	// Examples are example calls, rendered into the description and
	// checked against the input schema (see lint.go).
	Examples []Example `json:"examples,omitempty"`
}

// Variants is a tool's modes, selected by one argument.
type Variants struct {
	// Argument is the name of the selecting argument (e.g. action).
	Argument string `json:"argument"`
	// Description of the argument; the options are listed after it.
	Description string `json:"description,omitempty"`
	// Options are the modes by name (the enum values).
	Options map[string]Variant `json:"options"`
	// Default is the option that runs when the argument is omitted; without
	// it the argument is required.
	Default string `json:"default,omitempty"`
}

// Variant is one mode: a description for the model and an action.
type Variant struct {
	// Description says when to pick this mode.
	Description string            `json:"description"`
	Kubernetes  *KubernetesAction `json:"kubernetes,omitempty"`
	HTTP        *HTTPAction       `json:"http,omitempty"`
	Steps       []Step            `json:"steps,omitempty"`
	// Output overrides the tool's output for this mode.
	Output string `json:"output,omitempty"`
}

func (v *Variants) validate() []error {
	if v == nil {
		return nil
	}
	var errs []error
	if !validStepName.MatchString(v.Argument) {
		errs = append(errs, fmt.Errorf("spec.variants.argument %q must match %s", v.Argument, validStepName))
	}
	if len(v.Options) == 0 {
		errs = append(errs, errors.New("spec.variants.options must name at least one option"))
	}
	if _, ok := v.Options[v.Default]; v.Default != "" && !ok {
		errs = append(errs, fmt.Errorf("spec.variants.default %q is not one of the options", v.Default))
	}
	for name, option := range v.Options {
		path := "spec.variants.options[" + name + "]"
		if strings.TrimSpace(name) == "" {
			errs = append(errs, errors.New("spec.variants.options has an empty name"))
		}
		if strings.TrimSpace(option.Description) == "" {
			errs = append(errs, fmt.Errorf("%s.description is required", path))
		}
		set := 0
		for _, present := range []bool{option.Kubernetes != nil, option.HTTP != nil, len(option.Steps) > 0} {
			if present {
				set++
			}
		}
		switch {
		case set > 1:
			errs = append(errs, fmt.Errorf("%s: kubernetes, http and steps are mutually exclusive", path))
		case option.Kubernetes != nil:
			errs = append(errs, option.Kubernetes.validate()...)
		case option.HTTP != nil:
			errs = append(errs, option.HTTP.validate()...)
		case len(option.Steps) > 0:
			errs = append(errs, validateSteps(option.Steps)...)
		default:
			errs = append(errs, fmt.Errorf("%s needs one of kubernetes, http or steps", path))
		}
	}
	return errs
}

// optionNames returns the option names in order.
func (v *Variants) optionNames() []string {
	names := make([]string, 0, len(v.Options))
	for name := range v.Options {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// toolSpec is the option as a tool of its own: the parent's budget,
// requires, timeout and projections, this option's action.
func (v *Variant) toolSpec(parent ToolSpec) ToolSpec {
	spec := parent
	// The examples belong to the parent: they name the variants argument,
	// which the option alone does not declare.
	spec.Variants, spec.Examples = nil, nil
	spec.Kubernetes, spec.HTTP, spec.Steps = v.Kubernetes, v.HTTP, v.Steps
	if v.Output != "" {
		spec.Output = v.Output
	}
	return spec
}

// Limit is the output budget of a list-shaped result.
type Limit struct {
	// Default is the number of items returned when the model gives no limit.
	Default int `json:"default,omitempty"`
	// Max is the largest limit the model may ask for.
	Max int `json:"max,omitempty"`
	// Argument is the name of the limit argument (default "limit").
	Argument string `json:"argument,omitempty"`
	// Field names the key holding the list when the result is a map
	// (`metrics` in {'metrics': [...]}). A list result needs none, and a
	// map with exactly one list-valued key needs none either.
	Field string `json:"field,omitempty"`
	// NarrowWith names the arguments that narrow the result, for the
	// trailer; defaults to the declared labelSelector/fieldSelector arguments.
	NarrowWith []string `json:"narrowWith,omitempty"`
}

const (
	// DefaultLimit is the default of Limit.Default.
	DefaultLimit = 50
	// DefaultLimitMax is the default of Limit.Max.
	DefaultLimitMax = 500
	// LimitArgument is the default name of the limit argument.
	LimitArgument = "limit"
	// ContinueArgument is the name of the pagination argument.
	ContinueArgument = "continue"
	// DetailArgument is the name of the detail argument.
	DetailArgument = "detail"
)

// withDefaults returns the limit with its defaults filled in.
func (l *Limit) withDefaults() Limit {
	out := Limit{Default: DefaultLimit, Max: DefaultLimitMax, Argument: LimitArgument}
	if l == nil {
		return out
	}
	if l.Default > 0 {
		out.Default = l.Default
	}
	if l.Max > 0 {
		out.Max = l.Max
	}
	if l.Argument != "" {
		out.Argument = l.Argument
	}
	out.Field, out.NarrowWith = l.Field, l.NarrowWith
	return out
}

func (l *Limit) validate() []error {
	if l == nil {
		return nil
	}
	var errs []error
	if l.Default < 0 || l.Max < 0 {
		errs = append(errs, errors.New("spec.limit.default and spec.limit.max must be positive"))
	}
	filled := l.withDefaults()
	if filled.Default > filled.Max {
		errs = append(errs, fmt.Errorf("spec.limit.default %d exceeds spec.limit.max %d", filled.Default, filled.Max))
	}
	if !validStepName.MatchString(filled.Argument) {
		errs = append(errs, fmt.Errorf("spec.limit.argument %q must match %s", filled.Argument, validStepName))
	}
	return errs
}

// DefaultTimeout bounds a call of a Tool or Prompt that declares none: a
// backend that hangs must not hold the MCP call until the client gives up.
const DefaultTimeout = 60 * time.Second

// parseTimeout validates a declared timeout.
func parseTimeout(path, value string) (time.Duration, error) {
	if value == "" {
		return DefaultTimeout, nil
	}
	timeout, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s %q is not a duration: %w", path, value, err)
	}
	if timeout <= 0 {
		return 0, fmt.Errorf("%s %q must be positive", path, value)
	}
	return timeout, nil
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
	// response; without it the response Content-Type is used, and
	// application/octet-stream when the server does not say either.
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
// bearer token. Exactly one of URL, URLExpression or Service must be set.
type HTTPAction struct {
	Method string `json:"method,omitempty"`
	// URL is a fixed absolute base URL. URLExpression computes it at run time
	// as a CEL string over `steps` and `item`, for backends discovered from
	// cluster resources (a LokiStack's Route, a TempoStack's gateway); it must
	// not reference `args`, so the model never chooses where the caller's
	// token is sent. Service reaches an in-cluster Service through the API
	// server proxy.
	URL           string      `json:"url,omitempty"`
	URLExpression string      `json:"urlExpression,omitempty"`
	Service       *ServiceRef `json:"service,omitempty"`
	// Path is appended to the base URL. As a plain string, `{argument}`
	// placeholders are replaced with the URL-escaped tool argument of that
	// name; as an {argument|expression} reference the resolved value is
	// appended verbatim.
	Path            *Ref           `json:"path,omitempty"`
	Query           map[string]Ref `json:"query,omitempty"`
	QueryExpression string         `json:"queryExpression,omitempty"`
	// Headers maps header names to references; a header whose value resolves
	// to the empty string is not sent.
	Headers map[string]Ref `json:"headers,omitempty"`
	Body    *Body          `json:"body,omitempty"`
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
	for _, present := range []bool{t.Spec.Kubernetes != nil, t.Spec.HTTP != nil, len(t.Spec.Steps) > 0, t.Spec.Variants != nil} {
		if present {
			set++
		}
	}
	switch {
	case set > 1:
		errs = append(errs, errors.New("spec.kubernetes, spec.http, spec.steps and spec.variants are mutually exclusive"))
	case t.Spec.Kubernetes != nil:
		errs = append(errs, t.Spec.Kubernetes.validate()...)
	case t.Spec.HTTP != nil:
		errs = append(errs, t.Spec.HTTP.validate()...)
	case len(t.Spec.Steps) > 0:
		errs = append(errs, validateSteps(t.Spec.Steps)...)
	case t.Spec.Variants != nil:
		errs = append(errs, t.Spec.Variants.validate()...)
	default:
		errs = append(errs, errors.New("one of spec.kubernetes, spec.http, spec.steps or spec.variants is required"))
	}
	for i, req := range t.Spec.Requires {
		if req.APIVersion == "" || req.Kind == "" {
			errs = append(errs, fmt.Errorf("spec.requires[%d] needs apiVersion and kind", i))
		}
	}
	errs = append(errs, t.Spec.validateAnnotations()...)
	errs = append(errs, t.Spec.validatePassThrough()...)
	errs = append(errs, t.Spec.validateExamples()...)
	if _, err := parseTimeout("spec.timeout", t.Spec.Timeout); err != nil {
		errs = append(errs, err)
	}
	errs = append(errs, t.Spec.Limit.validate()...)
	if t.Spec.OutputFull != "" && t.Spec.Output == "" {
		errs = append(errs, errors.New("spec.outputFull needs spec.output (the concise projection)"))
	}
	return errors.Join(errs...)
}

// mutatingVerbs are the Kubernetes verbs that change cluster state.
var mutatingVerbs = map[string]bool{"create": true, "update": true, "patch": true, "delete": true}

// validateAnnotations checks the self-declared readOnly/destructive
// annotations against the Kubernetes verbs the Tool performs, which are
// static in the definition: the server's read-only and destructive filters
// trust the annotations, so an inconsistent declaration must fail at
// registration rather than expose a mutating tool as read-only. HTTP
// methods are not checked: a read may legitimately be a POST (Kiali).
func (t *ToolSpec) validateAnnotations() []error {
	var errs []error
	check := func(where string, action *KubernetesAction) {
		if action == nil {
			return
		}
		if t.ReadOnly && mutatingVerbs[action.Verb] {
			errs = append(errs, fmt.Errorf("%s performs a %s but spec.readOnly is true", where, action.Verb))
		}
		if action.Verb == "delete" && !t.Destructive {
			errs = append(errs, fmt.Errorf("%s performs a delete but spec.destructive is not true", where))
		}
	}
	check("spec.kubernetes", t.Kubernetes)
	for i, step := range t.Steps {
		check(fmt.Sprintf("spec.steps[%d] (%s)", i, step.Name), step.Kubernetes)
	}
	if t.Variants != nil {
		for _, name := range t.Variants.optionNames() {
			option := t.Variants.Options[name]
			check("spec.variants.options["+name+"]", option.Kubernetes)
			for i, step := range option.Steps {
				check(fmt.Sprintf("spec.variants.options[%s].steps[%d] (%s)", name, i, step.Name), step.Kubernetes)
			}
		}
	}
	return errs
}

// Mutates reports whether any step of the Prompt changes cluster state
// through the Kubernetes API. Prompts carry no annotations, so this is what
// the read-only filter uses; HTTP steps are not classified.
func (p *Prompt) Mutates() bool {
	for _, step := range p.Spec.Steps {
		if step.Kubernetes != nil && mutatingVerbs[step.Kubernetes.Verb] {
			return true
		}
	}
	return false
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
	if _, err := parseTimeout("spec.timeout", p.Spec.Timeout); err != nil {
		errs = append(errs, err)
	}
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
	set := 0
	if a.URL != "" {
		set++
	}
	if a.URLExpression != "" {
		set++
	}
	if a.Service != nil {
		set++
	}
	if set != 1 {
		errs = append(errs, errors.New("exactly one of spec.http.url, spec.http.urlExpression or spec.http.service is required"))
	}
	if a.URL != "" && !strings.HasPrefix(a.URL, "http://") && !strings.HasPrefix(a.URL, "https://") {
		errs = append(errs, fmt.Errorf("spec.http.url %q must be an absolute http(s) URL", a.URL))
	}
	if a.Service != nil && (a.Service.Namespace == "" || a.Service.Name == "") {
		errs = append(errs, errors.New("spec.http.service.namespace and spec.http.service.name are required"))
	}
	for key := range a.Headers {
		if reservedHeader(key) {
			errs = append(errs, fmt.Errorf("spec.http.headers may not set %s: the caller's identity is fixed by the server", key))
		}
	}
	if a.Body != nil {
		errs = append(errs, a.Body.validate("spec.http.body")...)
	}
	errs = append(errs, validateResponse("spec.http", a.Response)...)
	return errs
}

// reservedHeader reports whether a header carries identity: in service
// (API server proxy) mode a declared Authorization or Impersonate-* header
// would replace the caller, and Host/Cookie change where or as whom a
// request is served.
func reservedHeader(name string) bool {
	lower := strings.ToLower(name)
	switch lower {
	case "authorization", "proxy-authorization", "host", "cookie":
		return true
	}
	return strings.HasPrefix(lower, "impersonate-")
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
	if t.Variants != nil {
		for _, option := range t.Variants.Options {
			refs = append(refs, option.Kubernetes.refs()...)
			refs = append(refs, option.HTTP.refs()...)
			for _, step := range option.Steps {
				refs = append(refs, step.Kubernetes.refs()...)
				refs = append(refs, step.HTTP.refs()...)
			}
		}
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
	refs := []*Ref{a.Path}
	for key := range a.Query {
		ref := a.Query[key]
		refs = append(refs, &ref)
	}
	for key := range a.Headers {
		ref := a.Headers[key]
		refs = append(refs, &ref)
	}
	return refs
}
