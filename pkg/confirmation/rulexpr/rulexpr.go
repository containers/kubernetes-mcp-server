// Package rulexpr compiles and evaluates the CEL expressions of kube-level
// confirmation rules. It is a leaf package so that both configuration
// validation and rule matching can use it.
package rulexpr

import (
	"fmt"
	"sync"

	"github.com/google/cel-go/cel"
)

// Request is what a kube-level rule expression can inspect.
type Request struct {
	Verb      string
	Kind      string
	Group     string
	Version   string
	Name      string
	Namespace string
	// Object is the decoded request body for create/update/patch, or nil.
	Object map[string]any
}

var (
	envOnce sync.Once
	env     *cel.Env
	envErr  error
	cache   sync.Map // source -> cel.Program
)

func environment() (*cel.Env, error) {
	envOnce.Do(func() {
		env, envErr = cel.NewEnv(
			cel.Variable("verb", cel.StringType),
			cel.Variable("kind", cel.StringType),
			cel.Variable("group", cel.StringType),
			cel.Variable("version", cel.StringType),
			cel.Variable("name", cel.StringType),
			cel.Variable("namespace", cel.StringType),
			cel.Variable("object", cel.DynType),
		)
	})
	return env, envErr
}

// Compile checks an expression and caches the resulting program. The
// expression must evaluate to a boolean.
func Compile(source string) (cel.Program, error) {
	if program, ok := cache.Load(source); ok {
		return program.(cel.Program), nil
	}
	env, err := environment()
	if err != nil {
		return nil, err
	}
	ast, issues := env.Compile(source)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("invalid expression %q: %w", source, issues.Err())
	}
	if ast.OutputType() != cel.BoolType && ast.OutputType() != cel.DynType {
		return nil, fmt.Errorf("invalid expression %q: must evaluate to a boolean, got %s", source, ast.OutputType())
	}
	program, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("invalid expression %q: %w", source, err)
	}
	cache.Store(source, program)
	return program, nil
}

// Matches evaluates the expression against the request. An evaluation error
// (for example a field access on a null object outside a has() guard) is
// returned rather than treated as no match, so that callers guarding an
// action can fail closed.
func Matches(source string, request Request) (bool, error) {
	program, err := Compile(source)
	if err != nil {
		return false, err
	}
	var object any
	if request.Object != nil {
		object = request.Object
	}
	out, _, err := program.Eval(map[string]any{
		"verb": request.Verb, "kind": request.Kind, "group": request.Group, "version": request.Version,
		"name": request.Name, "namespace": request.Namespace, "object": object,
	})
	if err != nil {
		return false, fmt.Errorf("expression %q failed: %w", source, err)
	}
	matched, ok := out.Value().(bool)
	if !ok {
		return false, fmt.Errorf("expression %q produced %T, expected a boolean", source, out.Value())
	}
	return matched, nil
}
