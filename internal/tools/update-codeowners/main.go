package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/containers/kubernetes-mcp-server/internal/tools/ownership"
)

const outputPath = ".github/CODEOWNERS"

type pathRule struct {
	path      string
	approvers []string
	areas     []string
}

type codeownersRules struct {
	byPath  map[string]*pathRule
	ordered []*pathRule
	width   int
}

func main() {
	check := flag.Bool("check", false, "fail if CODEOWNERS differs from the generated file")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/tools/update-codeowners/main.go [-check]")
		os.Exit(1)
	}

	config, err := ownership.Load(ownership.InputPath)
	if err != nil {
		fail(err)
	}
	generated, err := render(config)
	if err != nil {
		fail(err)
	}

	if *check {
		current, err := os.ReadFile(outputPath)
		if err != nil {
			fail(err)
		}
		if !bytes.Equal(current, generated) {
			fail(fmt.Errorf("%s is out of date; run make update-codeowners", outputPath))
		}
		return
	}
	if err := os.WriteFile(outputPath, generated, 0o644); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func render(config ownership.Config) ([]byte, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	rules := collectOwnershipPaths(config)

	var out strings.Builder
	out.WriteString("# Generated from .github/ownership.json; run make update-codeowners to update.\n")
	out.WriteString("# Only approvers appear here; triage reads reviewers from the JSON.\n")
	out.WriteString("# Shared paths appear once, with approvers combined from the listed toolsets.\n\n")

	out.WriteString("# default\n")
	writeCodeownersRule(&out, rules.width, "*", ownership.Unique(config.DefaultApprovers))

	writeExclusivePathSections(&out, config.Areas, rules)
	writeOverlappingPathSections(&out, rules)

	return []byte(out.String()), nil
}

func collectOwnershipPaths(config ownership.Config) codeownersRules {
	rules := codeownersRules{byPath: make(map[string]*pathRule), width: 1}

	// Collect memberships before writing: a path shared by several areas needs
	// all their approvers, plus defaults (CODEOWNERS does not inherit owners).
	for i, area := range config.Areas {
		name := areaIdentifier(area, i)

		for _, path := range area.Paths {
			rule, exists := rules.byPath[path]
			if !exists {
				rule = &pathRule{path: path, approvers: ownership.Unique(config.DefaultApprovers)}
				rules.byPath[path] = rule
				rules.ordered = append(rules.ordered, rule)
				rules.width = max(rules.width, len(path))
			}

			rule.approvers = ownership.Unique(rule.approvers, area.Approvers)
			if !slices.Contains(rule.areas, name) {
				rule.areas = append(rule.areas, name)
			}
		}
	}

	return rules
}

func areaIdentifier(area ownership.Area, index int) string {
	if area.Toolset != "" {
		return area.Toolset
	}
	return fmt.Sprintf("area %d", index+1)
}

func writeCodeownersRule(out *strings.Builder, width int, path string, approvers []string) {
	fmt.Fprintf(out, "%-*s  %s\n", width, path, strings.Join(approvers, " "))
}

func writeExclusivePathSections(out *strings.Builder, areas []ownership.Area, rules codeownersRules) {
	for i, area := range areas {
		if area.Toolset != "" {
			fmt.Fprintf(out, "\n# toolset: %s\n", area.Toolset)
		} else {
			fmt.Fprintf(out, "\n# %s\n", areaIdentifier(area, i))
		}

		// Paths shared with another area are emitted in overlap sections below.
		for _, path := range area.Paths {
			rule := rules.byPath[path]
			if len(rule.areas) == 1 {
				writeCodeownersRule(out, rules.width, path, rule.approvers)
			}
		}
	}
}

func writeOverlappingPathSections(out *strings.Builder, rules codeownersRules) {
	// Group by exact area overlap, preserving first appearance in the JSON.
	shared := make(map[string][]*pathRule)
	var overlaps []string

	for _, rule := range rules.ordered {
		if len(rule.areas) < 2 {
			continue
		}

		key := strings.Join(rule.areas, ", ")
		if _, exists := shared[key]; !exists {
			overlaps = append(overlaps, key)
		}
		shared[key] = append(shared[key], rule)
	}

	for _, overlap := range overlaps {
		fmt.Fprintf(out, "\n# shared: %s\n", overlap)
		for _, rule := range shared[overlap] {
			writeCodeownersRule(out, rules.width, rule.path, rule.approvers)
		}
	}
}
