package dynamic

import (
	"fmt"
	"reflect"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
	"github.com/google/cel-go/ext"
	"google.golang.org/protobuf/types/known/structpb"
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

// newEnv builds the CEL environment shared by every expression:
//
//   - args: the tool arguments (map<string, dyn>)
//   - response: the decoded action response (dyn), null while building a request
//   - now: the current time (timestamp), e.g. int(now) for Unix seconds
//   - merge(a, b): a map with the entries of a overridden by those of b
//   - the cel-go strings, lists, math and two-variable comprehension
//     extensions (transformMapEntry, transformList, ...)
func newEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("args", stringDynMapType),
		cel.Variable("response", cel.DynType),
		cel.Variable("now", cel.TimestampType),
		cel.Function("merge",
			cel.Overload("merge_map_map", []*cel.Type{stringDynMapType, stringDynMapType}, stringDynMapType,
				cel.BinaryBinding(mergeMaps))),
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

func compileExpression(source string) (*expression, error) {
	env, err := newEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}
	ast, issues := env.Compile(source)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("invalid CEL expression %q: %w", source, issues.Err())
	}
	program, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("invalid CEL expression %q: %w", source, err)
	}
	return &expression{source: source, program: program}, nil
}

// eval runs the expression and returns a plain JSON-compatible Go value
// (map[string]any, []any, string, float64, bool or nil).
func (e *expression) eval(args map[string]any, response any) (any, error) {
	if args == nil {
		args = map[string]any{}
	}
	out, _, err := e.program.Eval(map[string]any{"args": args, "response": response, "now": time.Now()})
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
func (e *expression) evalStringMap(args map[string]any) (map[string]string, error) {
	value, err := e.eval(args, nil)
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
