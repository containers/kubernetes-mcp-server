package dynamic

import (
	"fmt"
	"reflect"

	"github.com/google/cel-go/cel"
	"google.golang.org/protobuf/types/known/structpb"
)

// expression is a compiled CEL program over the variables `args` and `response`.
type expression struct {
	source  string
	program cel.Program
}

var structpbValueType = reflect.TypeOf(&structpb.Value{})

func compileExpression(source string) (*expression, error) {
	env, err := cel.NewEnv(
		cel.Variable("args", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("response", cel.DynType),
	)
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
	out, _, err := e.program.Eval(map[string]any{"args": args, "response": response})
	if err != nil {
		return nil, fmt.Errorf("CEL expression %q failed: %w", e.source, err)
	}
	native, err := out.ConvertToNative(structpbValueType)
	if err != nil {
		return nil, fmt.Errorf("CEL expression %q produced a value that cannot be represented as JSON: %w", e.source, err)
	}
	return native.(*structpb.Value).AsInterface(), nil
}
