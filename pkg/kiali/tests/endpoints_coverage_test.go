package tests

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

// TestAllKialiEndpointsAreCoveredByToolImplementations verifies that every
// endpoint constant defined in endpoints.go is actually referenced by at least
// one tool implementation file. This prevents stale/dead endpoint definitions.
func TestAllKialiEndpointsAreCoveredByToolImplementations(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("unable to locate current test file path via runtime.Caller")
	}
	testsDir := filepath.Dir(thisFile)
	endpointsFile := filepath.Clean(filepath.Join(testsDir, "..", "..", "toolsets", "kiali", "tools", "endpoints.go"))
	toolsDir := filepath.Dir(endpointsFile)

	endpointNames, err := parseConstNames(endpointsFile)
	if err != nil {
		t.Fatalf("failed parsing endpoints from %s: %v", endpointsFile, err)
	}
	// KialiMCPPath is a base-path prefix used by other constants, not an endpoint itself.
	delete(endpointNames, "KialiMCPPath")

	if len(endpointNames) == 0 {
		t.Fatalf("no endpoint constants found in %s (unexpected)", endpointsFile)
	}

	usedNames, err := parseIdentUsesInDir(toolsDir, "endpoints.go", "endpoints_test.go")
	if err != nil {
		t.Fatalf("failed scanning tool files under %s: %v", toolsDir, err)
	}

	names := make([]string, 0, len(endpointNames))
	for name := range endpointNames {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			if !usedNames[name] {
				t.Fatalf("endpoint constant %q defined in endpoints.go is not referenced by any tool implementation in %s",
					name, toolsDir)
			}
		})
	}
}

func parseConstNames(filename string) (map[string]bool, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		return nil, err
	}

	names := map[string]bool{}
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			if len(vs.Values) == 0 {
				continue
			}
			for _, n := range vs.Names {
				if n != nil && n.Name != "" {
					names[n.Name] = true
				}
			}
		}
	}

	return names, nil
}

// parseIdentUsesInDir collects all identifier names used in Go files within
// dir, excluding files whose base names match excludeBases. This is used to
// verify that endpoint constants (defined in the same package) are referenced.
func parseIdentUsesInDir(dir string, excludeBases ...string) (map[string]bool, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}

	exclude := make(map[string]bool, len(excludeBases))
	for _, b := range excludeBases {
		exclude[b] = true
	}

	combined := map[string]bool{}
	for _, f := range matches {
		if exclude[filepath.Base(f)] {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			return nil, err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if ident, ok := n.(*ast.Ident); ok && ident.Name != "" {
				combined[ident.Name] = true
			}
			return true
		})
	}

	return combined, nil
}
