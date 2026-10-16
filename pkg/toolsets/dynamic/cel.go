package dynamic

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
	"github.com/google/cel-go/ext"
	"google.golang.org/protobuf/types/known/structpb"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/klogutil"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/containers/kubernetes-mcp-server/pkg/redact"
)

// expression is a compiled CEL program over the variables `args`,
// `response`, `steps`, `item`, `now` and `cluster`.
type expression struct {
	source  string
	ast     *cel.Ast
	program cel.Program
}

var (
	structpbValueType = reflect.TypeOf(&structpb.Value{})
	stringDynMapType  = cel.MapType(cel.StringType, cel.DynType)
)

// evalVars are the values bound to the CEL variables of one evaluation.
type evalVars struct {
	// ctx is the call's context; it carries the target the tool runs against
	// for functions that ask the cluster (serves). Nil outside a call.
	ctx      context.Context
	args     map[string]any
	response any
	steps    map[string]any
	item     any
}

// servesKey carries the cluster.serves(apiVersion, kind) implementation of
// the current target through the evaluation context.
type servesKey struct{}

// servesFunc reports whether the target serves a kind.
type servesFunc func(apiVersion, kind string) bool

// withServes binds cluster.serves(apiVersion, kind) to the target the call
// runs against: a kind is served when the target's RESTMapper resolves it,
// which is the same lookup a kubernetes action performs.
func withServes(ctx context.Context, client api.KubernetesClient) context.Context {
	return withServesFunc(ctx, func(apiVersion, kind string) bool {
		if client == nil {
			return false
		}
		gv, err := schema.ParseGroupVersion(apiVersion)
		if err != nil {
			return false
		}
		_, err = client.RESTMapper().RESTMapping(schema.GroupKind{Group: gv.Group, Kind: kind}, gv.Version)
		return err == nil
	})
}

func withServesFunc(ctx context.Context, serves servesFunc) context.Context {
	return context.WithValue(ctx, servesKey{}, serves)
}

// clusterType is the type of the `cluster` variable: an opaque value whose
// methods ask the target of the current call. The CEL environment is shared
// by every expression while the target is only known per call, so the
// methods are dispatched through the receiver trait instead of a binding
// declared in the environment.
var clusterType = cel.ObjectType("kubernetes-mcp-server.Cluster", traits.ReceiverType)

// clusterVal is the value of the `cluster` variable.
type clusterVal struct {
	serves servesFunc
}

// clusterFromContext returns the cluster value of the current call, or one
// whose methods fail outside a call.
func clusterFromContext(ctx context.Context) clusterVal {
	if serves, ok := ctx.Value(servesKey{}).(servesFunc); ok {
		return clusterVal{serves: serves}
	}
	return clusterVal{}
}

func (c clusterVal) Receive(function, _ string, args []ref.Val) ref.Val {
	if function != "serves" {
		return types.NewErr("no such method cluster.%s", function)
	}
	if c.serves == nil {
		return types.NewErr("cluster.serves is only available while a tool or prompt runs")
	}
	if len(args) != 2 {
		return types.NewErr("cluster.serves takes apiVersion and kind")
	}
	apiVersion, ok := args[0].(types.String)
	if !ok {
		return types.NewErr("cluster.serves: apiVersion must be a string")
	}
	kind, ok := args[1].(types.String)
	if !ok {
		return types.NewErr("cluster.serves: kind must be a string")
	}
	return types.Bool(c.serves(string(apiVersion), string(kind)))
}

func (c clusterVal) ConvertToNative(reflect.Type) (any, error) {
	return nil, fmt.Errorf("cluster cannot be converted to a native value")
}

func (c clusterVal) ConvertToType(typeValue ref.Type) ref.Val {
	if typeValue == types.TypeType {
		return clusterType
	}
	return types.NewErr("cluster cannot be converted to %s", typeValue.TypeName())
}

func (c clusterVal) Equal(other ref.Val) ref.Val {
	_, ok := other.(clusterVal)
	return types.Bool(ok)
}

func (c clusterVal) Type() ref.Type { return clusterType }

func (c clusterVal) Value() any { return c }

var (
	envOnce      sync.Once
	sharedEnv    *cel.Env
	sharedEnvErr error
)

// env returns the CEL environment shared by every expression. cel.Env is
// safe for concurrent use and building it (extensions, custom functions) is
// the expensive part of compiling, so it is done once per process rather
// than once per expression of every Tool on every re-apply.
func env() (*cel.Env, error) {
	envOnce.Do(func() { sharedEnv, sharedEnvErr = newEnv() })
	return sharedEnv, sharedEnvErr
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
		cel.Variable("cluster", clusterType),
		// cluster.serves(apiVersion, kind): dispatched to clusterVal.Receive.
		cel.Function("serves",
			cel.MemberOverload("cluster_serves_string_string", []*cel.Type{clusterType, cel.StringType, cel.StringType}, cel.BoolType)),
		// time(s): a time argument as Unix seconds, in the forms every tool
		// on the server accepts (docs/specs/tools-for-models.md).
		cel.Function("time",
			cel.Overload("time_string", []*cel.Type{cel.StringType}, cel.IntType,
				cel.UnaryBinding(timeArgument))),
		// seconds(s): a duration argument ("15m", "1h30m", "2d") in seconds.
		cel.Function("seconds",
			cel.Overload("seconds_string", []*cel.Type{cel.StringType}, cel.IntType,
				cel.UnaryBinding(secondsArgument))),
		cel.Function("sum",
			cel.Overload("sum_list", []*cel.Type{cel.ListType(cel.DynType)}, cel.DoubleType,
				cel.UnaryBinding(sumList))),
		cel.Function("range",
			cel.Overload("range_int", []*cel.Type{cel.IntType}, cel.ListType(cel.IntType),
				cel.UnaryBinding(rangeList)),
			// A count read from a response may still be a double (1.5e3).
			cel.Overload("range_double", []*cel.Type{cel.DoubleType}, cel.ListType(cel.IntType),
				cel.UnaryBinding(rangeList))),
		ext.Strings(),
		ext.Lists(),
		ext.Math(),
		ext.TwoVarComprehensions(),
		// cel.bind(name, value, expr): a local binding, so a definition can
		// name a sub-expression once instead of repeating it (CEL has no let).
		ext.Bindings(),
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
	native, err := toNative(v)
	if err != nil {
		return types.NewErr("cannot render value: %v", err)
	}
	text, err := render(native)
	if err != nil {
		return types.NewErr("cannot render value: %v", err)
	}
	return types.String(text)
}

// durationUnits are the units of a duration argument, in seconds.
var durationUnits = map[string]int64{"s": 1, "m": 60, "h": 3600, "d": 86400, "w": 7 * 86400, "y": 365 * 86400}

var durationPart = regexp.MustCompile(`^([0-9]+)(ms|s|m|h|d|w|y)`)

// parseSeconds parses "15m", "1h30m" or "2d" into seconds. Milliseconds
// are accepted (and truncated) so that a value a backend reports can be
// passed back.
func parseSeconds(value string) (int64, error) {
	rest := strings.TrimSpace(value)
	if rest == "" {
		return 0, fmt.Errorf("duration %q is empty", value)
	}
	var total int64
	for rest != "" {
		m := durationPart.FindStringSubmatch(rest)
		if m == nil {
			return 0, fmt.Errorf("duration %q must be a number with a unit (s, m, h, d, w, y), e.g. 30s, 15m, 1h30m, 2d", value)
		}
		n, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("duration %q is out of range", value)
		}
		if m[2] == "ms" {
			total += n / 1000
		} else {
			total += n * durationUnits[m[2]]
		}
		rest = rest[len(m[0]):]
	}
	return total, nil
}

// parseTime parses a time argument into Unix seconds: RFC 3339, Unix
// seconds, NOW, or NOW plus or minus a duration ("NOW-1h").
func parseTime(value string, now time.Time) (int64, error) {
	trimmed := strings.TrimSpace(value)
	upper := strings.ToUpper(trimmed)
	switch {
	case upper == "NOW":
		return now.Unix(), nil
	case strings.HasPrefix(upper, "NOW-") || strings.HasPrefix(upper, "NOW+"):
		offset, err := parseSeconds(trimmed[4:])
		if err != nil {
			return 0, fmt.Errorf("time %q: %w", value, err)
		}
		if upper[3] == '-' {
			return now.Unix() - offset, nil
		}
		return now.Unix() + offset, nil
	}
	if unix, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return unix, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		return t.Unix(), nil
	}
	return 0, fmt.Errorf("time %q must be RFC 3339 (2025-01-01T00:00:00Z), Unix seconds, NOW, or NOW minus a duration (NOW-1h)", value)
}

func timeArgument(v ref.Val) ref.Val {
	value, ok := v.(types.String)
	if !ok {
		return types.NewErr("time: argument must be a string")
	}
	unix, err := parseTime(string(value), time.Now())
	if err != nil {
		return types.NewErr("%s", err.Error())
	}
	return types.Int(unix)
}

func secondsArgument(v ref.Val) ref.Val {
	value, ok := v.(types.String)
	if !ok {
		return types.NewErr("seconds: argument must be a string")
	}
	seconds, err := parseSeconds(string(value))
	if err != nil {
		return types.NewErr("%s", err.Error())
	}
	return types.Int(seconds)
}

// sumList adds the numbers of a list as a double (ints and doubles mix;
// NaN and Inf propagate as they do in arithmetic). Series statistics need
// it because CEL comprehensions cannot fold.
func sumList(v ref.Val) ref.Val {
	list, ok := v.(traits.Lister)
	if !ok {
		return types.NewErr("sum: argument must be a list")
	}
	total := 0.0
	for it := list.Iterator(); it.HasNext() == types.True; {
		switch n := it.Next().(type) {
		case types.Double:
			total += float64(n)
		case types.Int:
			total += float64(n)
		case types.Uint:
			total += float64(n)
		default:
			return types.NewErr("sum: list elements must be numbers, got %s", n.Type().TypeName())
		}
	}
	return types.Double(total)
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
	env, err := env()
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
	return &expression{source: source, ast: ast, program: program}, nil
}

// references reports whether the expression reads the given top-level
// variable. It is a compile-time check, so a reference hidden behind a
// branch that never runs still counts.
func (e *expression) references(ident string) bool {
	found := false
	celast.PreOrderVisit(e.ast.NativeRep().Expr(), celast.NewExprVisitor(func(expr celast.Expr) {
		if expr.Kind() == celast.IdentKind && expr.AsIdent() == ident {
			found = true
		}
	}))
	return found
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
	ctx := vars.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	out, _, err := e.program.ContextEval(ctx, func() map[string]any {
		variables := map[string]any{"args": vars.args, "response": vars.response, "steps": vars.steps, "now": time.Now()}
		if vars.item != nil {
			// item binds only inside a forEach step: omitted otherwise, so an
			// expression that references it outside one fails instead of
			// silently seeing null (or the last element).
			variables["item"] = vars.item
		}
		variables["cluster"] = clusterFromContext(ctx)
		return variables
	}())
	if err != nil {
		// The model gets the CEL message, not the definition: the expression
		// source goes to the log, where the author looks.
		klogutil.FromContext(ctx).V(1).Info("dynamic toolset: expression failed", "expression", e.source, "error", err.Error())
		return nil, fmt.Errorf("expression failed: %w", err)
	}
	native, err := toNative(out)
	if err != nil {
		return nil, fmt.Errorf("CEL expression %q produced a value that cannot be represented as JSON: %w", e.source, err)
	}
	return native, nil
}

// toNative converts a CEL value to the plain Go value the engine passes
// between steps and renders: nil, bool, int64, float64, string, []any and
// map[string]any. Integers stay integers: a step that computes a Unix
// timestamp or a count must not hand the next step 1.7e+09, which is what
// the JSON (structpb) route produces. Values without a plain counterpart
// (timestamps, durations, bytes) still take the JSON route.
func toNative(v ref.Val) (any, error) {
	switch t := v.(type) {
	case types.Null:
		return nil, nil
	case types.Bool:
		return bool(t), nil
	case types.Int:
		return int64(t), nil
	case types.Uint:
		return int64(t), nil
	case types.Double:
		return float64(t), nil
	case types.String:
		return string(t), nil
	case traits.Lister:
		items := []any{}
		for it := t.Iterator(); it.HasNext() == types.True; {
			item, err := toNative(it.Next())
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		return items, nil
	case traits.Mapper:
		object := map[string]any{}
		for it := t.Iterator(); it.HasNext() == types.True; {
			key := it.Next()
			value, err := toNative(t.Get(key))
			if err != nil {
				return nil, err
			}
			object[fmt.Sprint(key.Value())] = value
		}
		return object, nil
	}
	native, err := v.ConvertToNative(structpbValueType)
	if err != nil {
		return nil, err
	}
	return native.(*structpb.Value).AsInterface(), nil
}

// evalStringMap evaluates an expression that must produce a map and returns
// its entries as query-parameter strings. Null and empty values are dropped;
// whole numbers print without a fractional part; a list value becomes a
// repeated parameter (Prometheus' match[], Alertmanager's filter).
func (e *expression) evalStringMap(vars evalVars) (map[string][]string, error) {
	value, err := e.eval(vars)
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("CEL expression %q must produce a map, got %T", e.source, value)
	}
	out := make(map[string][]string, len(object))
	for key, entry := range object {
		if values := scalarStrings(entry); len(values) > 0 {
			out[key] = values
		}
	}
	return out, nil
}

// scalarStrings renders a scalar as a one-element list and a list as its
// non-empty scalar elements.
func scalarStrings(value any) []string {
	items, ok := value.([]any)
	if !ok {
		if s := scalarString(value); s != "" {
			return []string{s}
		}
		return nil
	}
	var out []string
	for _, item := range items {
		if s := scalarString(item); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func scalarString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case int64:
		return fmt.Sprintf("%d", v)
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
// lists as YAML. `$${` renders a literal `${`, for shell-style text that is
// not an expression.
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
		if start > 0 && rest[start-1] == '$' {
			t.parts = append(t.parts, templatePart{literal: rest[:start-1] + "${"})
			rest = rest[start+2:]
			continue
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
