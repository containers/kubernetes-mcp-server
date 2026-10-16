package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"sigs.k8s.io/yaml"

	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/dynamic"
)

const checkLong = `Validate and lint Tool, Prompt and ToolSet manifests before applying them.

Reads multi-document YAML from files, from every .yaml/.yml file of a directory,
or from standard input with "-" (for example the output of helm template),
and runs what the server runs at registration: validation, CEL compilation
and the house-style lints of docs/specs/tools-for-models.md.

One line per resource, greppable by prefix:
  PASS: <file>: Tool <resource> (<mcp name>)
  LINT: <file>: Tool <resource>: <advice>
  FAIL: <file>: Tool <resource>: <error>
The exit status is 1 when a resource fails, and with --strict also when one
carries lints.`

const checkExamples = `  kubernetes-mcp-server check charts/dynamic-tools/tools/
  helm template my-tools ./my-chart | kubernetes-mcp-server check --strict -`

func newCheckCommand(streams genericiooptions.IOStreams) *cobra.Command {
	var strict bool
	cmd := &cobra.Command{
		Use:     "check [file|directory|-]...",
		Short:   "Validate and lint Tool, Prompt and ToolSet manifests",
		Long:    checkLong,
		Example: checkExamples,
		Args:    cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			summary, err := runCheck(streams, args)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(streams.Out, "checked %d resources: %d pass, %d with lints, %d failed\n", summary.checked, summary.passed, summary.linted, summary.failed)
			if summary.failed > 0 || (strict && summary.linted > 0) {
				return errors.New("check failed")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&strict, "strict", false, "Fail when a resource does not follow the house style (lints), not only when it is invalid")
	return cmd
}

type checkSummary struct{ checked, passed, linted, failed int }

func runCheck(streams genericiooptions.IOStreams, args []string) (checkSummary, error) {
	var summary checkSummary
	for _, arg := range args {
		inputs, err := checkInputs(streams.In, arg)
		if err != nil {
			return summary, err
		}
		for _, input := range inputs {
			summary.add(checkManifest(streams.Out, input.name, input.content))
		}
	}
	return summary, nil
}

type checkInput struct {
	name    string
	content []byte
}

// checkInputs resolves one argument: "-" is standard input, a directory is
// its .yaml and .yml files in name order, anything else one file.
func checkInputs(stdin io.Reader, arg string) ([]checkInput, error) {
	if arg == "-" {
		content, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("read standard input: %w", err)
		}
		return []checkInput{{name: "stdin", content: content}}, nil
	}
	info, err := os.Stat(arg)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		content, err := os.ReadFile(arg)
		if err != nil {
			return nil, err
		}
		return []checkInput{{name: arg, content: content}}, nil
	}
	// A directory means every YAML file it holds, in name order; both
	// extensions are read, and an empty result is an error rather than a
	// silent pass.
	var files []string
	for _, pattern := range []string{"*.yaml", "*.yml"} {
		matches, err := filepath.Glob(filepath.Join(arg, pattern))
		if err != nil {
			return nil, err
		}
		files = append(files, matches...)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s holds no .yaml or .yml files", arg)
	}
	sort.Strings(files)
	var inputs []checkInput
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, checkInput{name: file, content: content})
	}
	return inputs, nil
}

func (s *checkSummary) add(other checkSummary) {
	s.checked += other.checked
	s.passed += other.passed
	s.linted += other.linted
	s.failed += other.failed
}

// splitDocuments splits a multi-document YAML stream on document separator
// lines (--- alone, or --- followed by a comment, as helm renders), so a
// "----" rule inside a description does not split what it should not.
func splitDocuments(content string) []string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	docs, current := []string{}, strings.Builder{}
	separator := func(line string) (string, bool) {
		trimmed := strings.TrimRight(line, " \t")
		if trimmed == "---" {
			return "", true
		}
		if rest, ok := strings.CutPrefix(trimmed, "--- "); ok {
			return rest, true
		}
		return "", false
	}
	for _, line := range lines {
		if rest, ok := separator(line); ok {
			docs = append(docs, current.String())
			current.Reset()
			line = rest
		}
		current.WriteString(line)
		current.WriteString("\n")
	}
	return append(docs, current.String())
}

// checkManifest checks every Tool, Prompt and ToolSet document of one
// manifest the way the server does at registration, and prints one line
// per resource (and one per lint).
func checkManifest(out io.Writer, name string, content []byte) checkSummary {
	var summary checkSummary
	for _, doc := range splitDocuments(string(content)) {
		var header struct {
			Kind     string `json:"kind"`
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
		}
		if err := yaml.Unmarshal([]byte(doc), &header); err != nil || header.Kind == "" {
			continue
		}
		var exposed string
		var err error
		var lints []string
		switch header.Kind {
		case dynamic.Kind:
			var tool dynamic.Tool
			if err = yaml.Unmarshal([]byte(doc), &tool); err == nil {
				err = dynamic.Check(tool)
				exposed, lints = tool.ToolName(), tool.Lint()
			}
		case dynamic.PromptKind:
			var prompt dynamic.Prompt
			if err = yaml.Unmarshal([]byte(doc), &prompt); err == nil {
				err = dynamic.CheckPrompt(prompt)
				exposed = prompt.PromptName()
			}
		case dynamic.ToolSetKind:
			var toolset dynamic.ToolSet
			if err = yaml.Unmarshal([]byte(doc), &toolset); err == nil {
				err = toolset.Validate()
				exposed = toolset.ToolsetName()
			}
		default:
			continue
		}
		summary.checked++
		where := fmt.Sprintf("%s: %s %s", name, header.Kind, header.Metadata.Name)
		switch {
		case err != nil:
			summary.failed++
			for _, line := range strings.Split(err.Error(), "\n") {
				_, _ = fmt.Fprintf(out, "FAIL: %s: %s\n", where, line)
			}
		case len(lints) > 0:
			summary.linted++
			for _, lint := range lints {
				_, _ = fmt.Fprintf(out, "LINT: %s: %s\n", where, lint)
			}
		default:
			summary.passed++
			_, _ = fmt.Fprintf(out, "PASS: %s (%s)\n", where, exposed)
		}
	}
	return summary
}
