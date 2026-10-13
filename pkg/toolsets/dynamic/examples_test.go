package dynamic

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
	"sigs.k8s.io/yaml"
)

// ExamplesSuite keeps every Tool definition shipped under examples/dynamic-tools
// valid: hand-written examples, exporter output and the rendered Helm charts
// must all parse, validate and compile their CEL expressions.
type ExamplesSuite struct {
	suite.Suite
	root string
}

func (s *ExamplesSuite) SetupSuite() {
	s.root = filepath.Join("..", "..", "..", "examples", "dynamic-tools")
}

func (s *ExamplesSuite) checkDocuments(name string, content []byte) int {
	count := 0
	for _, doc := range strings.Split(string(content), "\n---") {
		var header map[string]any
		if err := yaml.Unmarshal([]byte(doc), &header); err != nil || header == nil {
			continue
		}
		switch header["kind"] {
		case Kind:
			var tool Tool
			s.Require().NoError(yaml.Unmarshal([]byte(doc), &tool), "%s: invalid YAML", name)
			s.Require().NoError(Check(tool), "%s: tool %s", name, tool.Metadata.Name)
		case PromptKind:
			var prompt Prompt
			s.Require().NoError(yaml.Unmarshal([]byte(doc), &prompt), "%s: invalid YAML", name)
			s.Require().NoError(CheckPrompt(prompt), "%s: prompt %s", name, prompt.Metadata.Name)
		default:
			continue
		}
		count++
		s.Equal(Group+"/"+Version, header["apiVersion"], "%s", name)
	}
	return count
}

func (s *ExamplesSuite) TestHandWrittenExamplesAreValid() {
	files, err := filepath.Glob(filepath.Join(s.root, "tools", "*.yaml"))
	s.Require().NoError(err)
	s.Require().NotEmpty(files)
	for _, file := range files {
		s.Run(filepath.Base(file), func() {
			raw, err := os.ReadFile(file)
			s.Require().NoError(err)
			s.Equal(1, s.checkDocuments(file, raw))
		})
	}
}

func (s *ExamplesSuite) TestExportedManifestsAreValid() {
	files, err := filepath.Glob(filepath.Join(s.root, "converted", "*.yaml"))
	s.Require().NoError(err)
	s.Require().NotEmpty(files, "run go run ./examples/dynamic-tools/hack/export")
	for _, file := range files {
		s.Run(filepath.Base(file), func() {
			raw, err := os.ReadFile(file)
			s.Require().NoError(err)
			s.Positive(s.checkDocuments(file, raw))
		})
	}
}

func (s *ExamplesSuite) TestChartsRenderValidTools() {
	helm, err := exec.LookPath("helm")
	if err != nil {
		helm = filepath.Join("..", "..", "..", "_output", "tools", "bin", "helm")
		if _, statErr := os.Stat(helm); statErr != nil {
			s.T().Skip("helm not available (make helm)")
		}
	}
	charts, err := filepath.Glob(filepath.Join(s.root, "charts", "*"))
	s.Require().NoError(err)
	s.Require().NotEmpty(charts)
	for _, chart := range charts {
		s.Run(filepath.Base(chart), func() {
			out, err := exec.Command(helm, "template", "x", chart, "--set", "url=https://example.invalid").CombinedOutput()
			s.Require().NoError(err, string(out))
			s.Positive(s.checkDocuments(chart, out))
		})
	}
}

func TestExamples(t *testing.T) {
	suite.Run(t, new(ExamplesSuite))
}
