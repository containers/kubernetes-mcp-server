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

func TestAllKialiEndpointsAreCoveredByBackendContractTests(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("unable to locate current test file path via runtime.Caller")
	}
	testsDir := filepath.Dir(thisFile)
	endpointsFile := filepath.Clean(filepath.Join(testsDir, "..", "endpoints.go"))

	endpointNames, err := parseConstNames(endpointsFile)
	if err != nil {
		t.Fatalf("failed parsing endpoints from %s: %v", endpointsFile, err)
	}
	if len(endpointNames) == 0 {
		t.Fatalf("no endpoints found in %s (unexpected)", endpointsFile)
	}
	usedNames, err := parseKialiSelectorUsesInDir(filepath.Join(testsDir, "backend"), filepath.Base(thisFile))
	if err != nil {
		t.Fatalf("failed parsing backend tests under %s: %v", testsDir, err)
	}

	names := make([]string, 0, len(endpointNames))
	for name := range endpointNames {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			if !usedNames[name] {
				t.Fatalf("endpoint constant %q is not referenced by any tests under %s (add a test that references kiali.%s)",
					name, testsDir, name)
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

			// Only consider consts that are assigned string literals in endpoints.go.
			// (This matches the intended contract for API endpoints.)
			if len(vs.Values) == 0 {
				continue
			}
			if bl, ok := vs.Values[0].(*ast.BasicLit); !ok || bl.Kind != token.STRING {
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

func parseKialiSelectorUses(filename string) (map[string]bool, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		return nil, err
	}

	used := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "kiali" {
			return true
		}
		if sel.Sel != nil && sel.Sel.Name != "" {
			used[sel.Sel.Name] = true
		}
		return true
	})

	return used, nil
}

func parseKialiSelectorUsesInDir(dir, excludeBase string) (map[string]bool, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return map[string]bool{}, nil
	}

	combined := map[string]bool{}
	for _, f := range matches {
		if filepath.Base(f) == excludeBase {
			continue
		}
		used, err := parseKialiSelectorUses(f)
		if err != nil {
			return nil, err
		}
		for k := range used {
			combined[k] = true
		}
	}

	return combined, nil
}
