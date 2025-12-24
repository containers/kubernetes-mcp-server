package main

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <endpoints.go> <api_test.go>\n", os.Args[0])
		os.Exit(1)
	}

	endpointsFile := os.Args[1]
	testFile := os.Args[2]

	// Extract endpoint constants from endpoints.go
	endpoints, err := extractEndpoints(endpointsFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error extracting endpoints: %v\n", err)
		os.Exit(1)
	}

	// Extract test function names from api_test.go
	tests, err := extractTests(testFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error extracting tests: %v\n", err)
		os.Exit(1)
	}

	// Check which endpoints are missing tests
	missing := findMissingTests(endpoints, tests)

	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "\n❌ ERROR: The following endpoints are missing contract tests:\n\n")
		for _, endpoint := range missing {
			fmt.Fprintf(os.Stderr, "  - %s\n", endpoint)
		}
		fmt.Fprintf(os.Stderr, "\nPlease add tests for these endpoints in %s\n\n", testFile)
		os.Exit(1)
	}

	fmt.Println("✅ All endpoints have corresponding contract tests")
}

// extractEndpoints extracts endpoint constant names from endpoints.go
func extractEndpoints(filename string) ([]string, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var endpoints []string
	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}

		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			for _, name := range valueSpec.Names {
				// Only include endpoints (those ending with "Endpoint")
				if strings.HasSuffix(name.Name, "Endpoint") {
					endpoints = append(endpoints, name.Name)
				}
			}
		}
	}

	return endpoints, nil
}

// extractTests extracts test function names from api_test.go
// It looks for functions that use the endpoint constants
func extractTests(filename string) (map[string]bool, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	tests := make(map[string]bool)

	// Read the file line by line to find test functions and their endpoint usage
	scanner := bufio.NewScanner(strings.NewReader(string(content)))

	// Pattern to match endpoint constant usage: kiali.EndpointName
	endpointPattern := regexp.MustCompile(`kiali\.(\w+Endpoint)`)

	for scanner.Scan() {
		line := scanner.Text()
		matches := endpointPattern.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) > 1 {
				endpointName := match[1]
				tests[endpointName] = true
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return tests, nil
}

// findMissingTests finds endpoints that don't have corresponding tests
func findMissingTests(endpoints []string, tests map[string]bool) []string {
	var missing []string
	for _, endpoint := range endpoints {
		if !tests[endpoint] {
			missing = append(missing, endpoint)
		}
	}
	return missing
}
