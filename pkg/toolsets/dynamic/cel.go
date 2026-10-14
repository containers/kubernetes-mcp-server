package dynamic

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
	"github.com/google/cel-go/ext"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/containers/kubernetes-mcp-server/pkg/redact"
)

// expression is a compiled CEL program over the variables `args`, `response`
// and `now`.
type expression struct {
	source  string
	program cel.Program
}

var (
	structpbValueType = reflect.TypeOf(&structpb.Value{})
	stringDynMapType  = cel.MapType(cel.StringType, cel.DynType)
)

// evalVars are the values bound to the CEL variables of one evaluation.
type evalVars struct {
	args     map[string]any
	response any
	steps    map[string]any
	item     any
}

// newEnv builds the CEL environment shared by every expression:
//
//   - args: the tool arguments (map<string, dyn>)
//   - response: the decoded action response (dyn), null while building a request
//   - steps: results of the previous steps by name (map<string, dyn>)
//   - item: the current element of a forEach step (dyn)
//   - now: the current time (timestamp), e.g. int(now) for Unix seconds
//   - merge(a, b): a map with the entries of a overridden by those of b
//   - range(n): the list [0, 1, ..., n-1] (n <= MaxRange), to walk two lists
//     in parallel
//   - the cel-go strings, lists, math and two-variable comprehension
//     extensions (transformMapEntry, transformList, flatten, join, ...)
func newEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("args", stringDynMapType),
		cel.Variable("response", cel.DynType),
		cel.Variable("steps", stringDynMapType),
		cel.Variable("item", cel.DynType),
		cel.Variable("now", cel.TimestampType),
		cel.Function("merge",
			cel.Overload("merge_map_map", []*cel.Type{stringDynMapType, stringDynMapType}, stringDynMapType,
				cel.BinaryBinding(mergeMaps))),
		cel.Function("yaml",
			cel.Overload("yaml_dyn", []*cel.Type{cel.DynType}, cel.StringType,
				cel.UnaryBinding(func(v ref.Val) ref.Val { return renderValue(v, output.MarshalYaml) }))),
		cel.Function("json",
			cel.Overload("json_dyn", []*cel.Type{cel.DynType}, cel.StringType,
				cel.UnaryBinding(func(v ref.Val) ref.Val {
					return renderValue(v, func(value any) (string, error) {
						raw, err := json.Marshal(value)
						return string(raw), err
					})
				}))),
		cel.Function("redact",
			cel.Overload("redact_string", []*cel.Type{cel.StringType}, cel.StringType,
				cel.UnaryBinding(func(v ref.Val) ref.Val {
					text, ok := v.(types.String)
					if !ok {
						return types.NewErr("redact: argument must be a string")
					}
					return types.String(redact.Secrets(string(text)))
				}))),
		cel.Function("range",
			cel.Overload("range_int", []*cel.Type{cel.IntType}, cel.ListType(cel.IntType),
				cel.UnaryBinding(rangeList)),
			// Step results travel through JSON, so their numbers are doubles.
			cel.Overload("range_double", []*cel.Type{cel.DoubleType}, cel.ListType(cel.IntType),
				cel.UnaryBinding(rangeList))),
		ext.Strings(),
		ext.Lists(),
		ext.Math(),
		ext.TwoVarComprehensions(),
	)
}

func mergeMaps(lhs, rhs ref.Val) ref.Val {
	out := map[ref.Val]ref.Val{}
	for _, value := range []ref.Val{lhs, rhs} {
		mapper, ok := value.(traits.Mapper)
		if !ok {
			return types.NewErr("merge: arguments must be maps")
		}
		it := mapper.Iterator()
		for it.HasNext() == types.True {
			key := it.Next()
			out[key] = mapper.Get(key)
		}
	}
	return types.NewRefValMap(types.DefaultTypeAdapter, out)
}

// renderValue converts a CEL value to its JSON-compatible form and renders
// it with the given serializer.
func renderValue(v ref.Val, render func(any) (string, error)) ref.Val {
	native, err := v.ConvertToNative(structpbValueType)
	if err != nil {
		return types.NewErr("cannot render value: %v", err)
	}
	text, err := render(native.(*structpb.Value).AsInterface())
	if err != nil {
		return types.NewErr("cannot render value: %v", err)
	}
	return types.String(text)
}

func rangeList(n ref.Val) ref.Val {
	var count types.Int
	switch v := n.(type) {
	case types.Int:
		count = v
	case types.Double:
		count = types.Int(v)
	default:
		return types.NewErr("range: argument must be a number")
	}
	if count < 0 {
		return types.NewErr("range: argument must be non-negative")
	}
	if count > MaxRange {
		return types.NewErr("range: at most %d items are allowed, got %d", MaxRange, int64(count))
	}
	items := make([]any, 0, count)
	for i := int64(0); i < int64(count); i++ {
		items = append(items, i)
	}
	return types.DefaultTypeAdapter.NativeToValue(items)
}

func compileExpression(source string) (*expression, error) {
	env, err := newEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}
	ast, issues := env.Compile(source)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("invalid CEL expression %q: %w", source, issues.Err())
	}
	// Tool arguments and responses are attacker-influenced, so every
	// evaluation is bounded: cel-go aborts once the tracked cost exceeds the
	// limit instead of letting a comprehension run unbounded.
	program, err := env.Program(ast, cel.CostLimit(CELCostLimit))
	if err != nil {
		return nil, fmt.Errorf("invalid CEL expression %q: %w", source, err)
	}
	return &expression{source: source, program: program}, nil
}

// eval runs the expression and returns a plain JSON-compatible Go value
// (map[string]any, []any, string, float64, bool or nil).
func (e *expression) eval(vars evalVars) (any, error) {
	if vars.args == nil {
		vars.args = map[string]any{}
	}
	if vars.steps == nil {
		vars.steps = map[string]any{}
	}
	out, _, err := e.program.Eval(map[string]any{
		"args": vars.args, "response": vars.response, "steps": vars.steps, "item": vars.item, "now": time.Now(),
	})
	if err != nil {
		return nil, fmt.Errorf("CEL expression %q failed: %w", e.source, err)
	}
	native, err := out.ConvertToNative(structpbValueType)
	if err != nil {
		return nil, fmt.Errorf("CEL expression %q produced a value that cannot be represented as JSON: %w", e.source, err)
	}
	return native.(*structpb.Value).AsInterface(), nil
}

// evalStringMap evaluates an expression that must produce a map and returns
// its entries as query-parameter strings. Null and empty values are dropped;
// whole numbers print without a fractional part.
func (e *expression) evalStringMap(vars evalVars) (map[string]string, error) {
	value, err := e.eval(vars)
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("CEL expression %q must produce a map, got %T", e.source, value)
	}
	out := make(map[string]string, len(object))
	for key, entry := range object {
		if s := scalarString(entry); s != "" {
			out[key] = s
		}
	}
	return out, nil
}

func scalarString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprint(v)
	case bool:
		return fmt.Sprint(v)
	default:
		return fmt.Sprint(v)
	}
}

// template is a text with `${expression}` placeholders. Strings render as
// they are, null as empty, numbers and booleans as scalars, and maps or
// lists as YAML.
type template struct {
	source string
	parts  []templatePart
}

type templatePart struct {
	literal string
	expr    *expression
}

func parseTemplate(source string) (*template, error) {
	t := &template{source: source}
	rest := source
	for {
		start := strings.Index(rest, "${")
		if start < 0 {
			t.parts = append(t.parts, templatePart{literal: rest})
			return t, nil
		}
		t.parts = append(t.parts, templatePart{literal: rest[:start]})
		depth, end := 0, -1
	scan:
		for i := start + 2; i < len(rest); i++ {
			switch rest[i] {
			case '{':
				depth++
			case '}':
				if depth == 0 {
					end = i
					break scan
				}
				depth--
			}
		}
		if end < 0 {
			return nil, fmt.Errorf("template has an unterminated ${ placeholder: %q", source)
		}
		expr, err := compileExpression(rest[start+2 : end])
		if err != nil {
			return nil, err
		}
		t.parts = append(t.parts, templatePart{expr: expr})
		rest = rest[end+1:]
	}
}

func (t *template) render(vars evalVars) (string, error) {
	var out strings.Builder
	for _, part := range t.parts {
		if part.expr == nil {
			out.WriteString(part.literal)
			continue
		}
		value, err := part.expr.eval(vars)
		if err != nil {
			return "", err
		}
		switch v := value.(type) {
		case nil:
		case string:
			out.WriteString(v)
		case map[string]any, []any:
			text, err := output.MarshalYaml(v)
			if err != nil {
				return "", err
			}
			out.WriteString(text)
		default:
			out.WriteString(scalarString(v))
		}
	}
	return out.String(), nil
}
