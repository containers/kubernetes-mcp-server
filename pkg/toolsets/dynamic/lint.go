package dynamic

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

// Example is one example call of a Tool (api.Example): rendered into its
// description and checked against the input schema.
type Example = api.Example

// property is what the lints and the example checks need of a declared
// input schema property.
type property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Enum        []any  `json:"enum"`
}

// declaredArguments parses the input schema into its properties and
// required names, tolerating anything it does not understand.
func (t *ToolSpec) declaredArguments() (map[string]property, []string) {
	if len(t.InputSchema) == 0 {
		return nil, nil
	}
	var schema struct {
		Properties map[string]property `json:"properties"`
		Required   []string            `json:"required"`
	}
	if json.Unmarshal(t.InputSchema, &schema) != nil {
		return nil, nil
	}
	return schema.Properties, schema.Required
}

// engineArguments are the arguments the engine adds to the schema (limit,
// continue, detail, the variants argument), which an example may use.
func (t *ToolSpec) engineArguments() map[string][]any {
	out := map[string][]any{}
	if t.Limit != nil {
		out[t.Limit.withDefaults().Argument] = nil
		out[ContinueArgument] = nil
	}
	if t.OutputFull != "" {
		out[DetailArgument] = []any{"concise", "full"}
	}
	if t.Variants != nil {
		var options []any
		for _, name := range t.Variants.optionNames() {
			options = append(options, name)
		}
		out[t.Variants.Argument] = options
	}
	return out
}

// validateExamples checks that every example names declared arguments,
// gives the required ones and respects their enums, so an example the
// description shows is a call the tool accepts.
func (t *ToolSpec) validateExamples() []error {
	var errs []error
	properties, required := t.declaredArguments()
	engine := t.engineArguments()
	if t.Variants != nil && t.Variants.Default == "" {
		required = append(required, t.Variants.Argument)
	}
	for i, example := range t.Examples {
		where := fmt.Sprintf("spec.examples[%d]", i)
		if strings.TrimSpace(example.Summary) == "" {
			errs = append(errs, errors.New(where+".summary is required"))
		}
		for _, name := range required {
			if _, ok := example.Arguments[name]; !ok {
				errs = append(errs, fmt.Errorf("%s lacks the required argument %q", where, name))
			}
		}
		if properties == nil && len(engine) == 0 {
			continue
		}
		for name, value := range example.Arguments {
			declared, known := properties[name]
			enum := declared.Enum
			if !known {
				if enum, known = engine[name]; !known {
					errs = append(errs, fmt.Errorf("%s.arguments.%s is not a declared argument", where, name))
					continue
				}
			}
			if len(enum) > 0 && !enumContains(enum, value) {
				errs = append(errs, fmt.Errorf("%s.arguments.%s is %v, not one of %v", where, name, value, enum))
			}
		}
	}
	return errs
}

func enumContains(enum []any, value any) bool {
	for _, allowed := range enum {
		if fmt.Sprint(allowed) == fmt.Sprint(value) {
			return true
		}
	}
	return false
}

// passThrough is the CEL that forwards the model's arguments wholesale.
func passThrough(expression string) bool {
	return strings.TrimSpace(expression) == "args"
}

// validatePassThrough refuses a kubernetes action whose query or body is
// the model's arguments as they are: the tool's contract would be whatever
// the backend accepts, which neither the model nor the RBAC metadata can
// see (docs/specs/tools-for-models.md).
func (t *ToolSpec) validatePassThrough() []error {
	var errs []error
	check := func(where string, action *KubernetesAction) {
		if action == nil {
			return
		}
		if passThrough(action.QueryExpression) {
			errs = append(errs, fmt.Errorf("%s.queryExpression forwards the model's arguments wholesale (args); declare the parameters the tool means", where))
		}
		if action.Body != nil && passThrough(action.Body.Expression) {
			errs = append(errs, fmt.Errorf("%s.body.expression forwards the model's arguments wholesale (args); declare the fields the tool means", where))
		}
	}
	t.eachKubernetesAction(check)
	return errs
}

// eachKubernetesAction visits the kubernetes action of the Tool, of each
// step, and of each variant option and its steps.
func (t *ToolSpec) eachKubernetesAction(visit func(where string, action *KubernetesAction)) {
	visit("spec.kubernetes", t.Kubernetes)
	for i, step := range t.Steps {
		visit(fmt.Sprintf("spec.steps[%d] (%s)", i, step.Name), step.Kubernetes)
	}
	if t.Variants != nil {
		for _, name := range t.Variants.optionNames() {
			option := t.Variants.Options[name]
			visit("spec.variants.options["+name+"]", option.Kubernetes)
			for i, step := range option.Steps {
				visit(fmt.Sprintf("spec.variants.options[%s].steps[%d] (%s)", name, i, step.Name), step.Kubernetes)
			}
		}
	}
}

// MaxToolsPerToolSet is the number of tools above which a ToolSet is
// warned that the model's selection accuracy degrades.
const MaxToolsPerToolSet = 12

// Lint returns the advice of docs/specs/tools-for-models.md the Tool does
// not follow: api.LintTool's, and the engine's own. None of it makes the
// Tool invalid: the status condition carries the list, the shipped charts
// are tested to have none.
func (t *Tool) Lint() []string {
	tool := api.Tool{Name: t.ToolName(), Description: t.Spec.Description, Examples: t.Spec.Examples}
	if len(t.Spec.InputSchema) > 0 {
		var schema jsonschema.Schema
		if json.Unmarshal(t.Spec.InputSchema, &schema) == nil {
			tool.InputSchema = &schema
		}
	}
	var lints []string
	unbounded := func(where string, action *KubernetesAction, output string) {
		if action != nil && action.Verb == "list" && output == "" && t.Spec.Limit == nil {
			lints = append(lints, where+" lists without output or limit: the result is the default projection of every item")
		}
	}
	unbounded("spec.kubernetes", t.Spec.Kubernetes, t.Spec.Output)
	if t.Spec.Variants != nil {
		for _, name := range t.Spec.Variants.optionNames() {
			option := t.Spec.Variants.Options[name]
			output := option.Output
			if output == "" {
				output = t.Spec.Output
			}
			unbounded("spec.variants.options["+name+"]", option.Kubernetes, output)
		}
	}
	return append(api.LintTool(tool), lints...)
}
