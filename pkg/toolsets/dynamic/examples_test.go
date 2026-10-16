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

// ExamplesSuite keeps every Tool definition shipped under charts/dynamic-tools
// valid: the hand-written manifests and the rendered Helm charts must all
// parse, validate and compile their CEL expressions.
type ExamplesSuite struct {
	suite.Suite
	root string
}

func (s *ExamplesSuite) SetupSuite() {
	s.root = filepath.Join("..", "..", "..", "charts", "dynamic-tools")
}

// charts returns every chart directory under the suite root, skipping the
// hand-written tools directory and the loose files (the CRD, the README)
// that live next to the charts.
func (s *ExamplesSuite) charts() []string {
	entries, err := os.ReadDir(s.root)
	s.Require().NoError(err)
	var charts []string
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "tools" {
			charts = append(charts, filepath.Join(s.root, entry.Name()))
		}
	}
	s.Require().NotEmpty(charts)
	return charts
}

func (s *ExamplesSuite) checkDocuments(name string, content []byte) int {
	count := 0
	for _, doc := range strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n---") {
		var header map[string]any
		if err := yaml.Unmarshal([]byte(doc), &header); err != nil || header == nil {
			continue
		}
		switch header["kind"] {
		case Kind:
			var tool Tool
			s.Require().NoError(yaml.Unmarshal([]byte(doc), &tool), "%s: invalid YAML", name)
			s.Require().NoError(Check(tool), "%s: tool %s", name, tool.Metadata.Name)
			s.Empty(tool.Lint(), "%s: tool %s follows docs/specs/tools-for-models.md", name, tool.ToolName())
		case PromptKind:
			var prompt Prompt
			s.Require().NoError(yaml.Unmarshal([]byte(doc), &prompt), "%s: invalid YAML", name)
			s.Require().NoError(CheckPrompt(prompt), "%s: prompt %s", name, prompt.Metadata.Name)
		case ToolSetKind:
			var toolset ToolSet
			s.Require().NoError(yaml.Unmarshal([]byte(doc), &toolset), "%s: invalid YAML", name)
			s.Require().NoError(toolset.Validate(), "%s: toolset %s", name, toolset.Metadata.Name)
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

// chartHelm returns the helm executable, or skips the test when none is
// available.
func (s *ExamplesSuite) chartHelm() string {
	if helm, err := exec.LookPath("helm"); err == nil {
		return helm
	}
	helm := filepath.Join("..", "..", "..", "_output", "tools", "bin", "helm")
	if _, err := os.Stat(helm); err != nil {
		s.T().Skip("helm not available (make helm)")
	}
	return helm
}

func (s *ExamplesSuite) TestChartsRenderValidTools() {
	helm := s.chartHelm()
	charts := s.charts()
	for _, chart := range charts {
		s.Run(filepath.Base(chart), func() {
			out, err := exec.Command(helm, "template", "x", chart, "--set", "url=https://example.invalid").CombinedOutput()
			s.Require().NoError(err, string(out))
			s.Positive(s.checkDocuments(chart, out))
		})
	}
}

func (s *ExamplesSuite) TestChartsShipAToolSetAndTagEveryToolAndPrompt() {
	// Every chart is a family: one ToolSet resource, and every Tool and
	// Prompt names it and carries its prefix (docs/specs/tools-for-models.md).
	helm := s.chartHelm()
	for _, chart := range s.charts() {
		s.Run(filepath.Base(chart), func() {
			out, err := exec.Command(helm, "template", "x", chart, "--set", "url=https://example.invalid", "--set", "alertmanagerUrl=https://example.invalid").CombinedOutput()
			s.Require().NoError(err, string(out))
			var toolsets []ToolSet
			var tools []Tool
			var prompts []Prompt
			for _, doc := range strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n---") {
				var header map[string]any
				if err := yaml.Unmarshal([]byte(doc), &header); err != nil || header == nil {
					continue
				}
				switch header["kind"] {
				case ToolSetKind:
					var toolset ToolSet
					s.Require().NoError(yaml.Unmarshal([]byte(doc), &toolset))
					toolsets = append(toolsets, toolset)
				case Kind:
					var tool Tool
					s.Require().NoError(yaml.Unmarshal([]byte(doc), &tool))
					tools = append(tools, tool)
				case PromptKind:
					var prompt Prompt
					s.Require().NoError(yaml.Unmarshal([]byte(doc), &prompt))
					prompts = append(prompts, prompt)
				}
			}
			s.Require().Len(toolsets, 1, "one ToolSet per chart")
			family := toolsets[0]
			s.NotEmpty(family.Spec.Prefix, "the family declares the prefix its tools are named with")
			s.NotEmpty(strings.TrimSpace(family.Spec.Instructions), "the family carries its workflow guidance")
			s.LessOrEqual(len(tools), 12, "a family beyond a dozen tools is hard to tell apart in tools/list (docs/specs/tools-for-models.md)")
			for _, tool := range tools {
				s.Equal(family.Metadata.Name, tool.Spec.ToolSet, "tool %s", tool.Metadata.Name)
				s.True(strings.HasPrefix(tool.ToolName(), family.Spec.Prefix+"_"), "tool %s must carry the %s_ prefix", tool.ToolName(), family.Spec.Prefix)
				s.NoError(tool.Validate(), "tool %s", tool.Metadata.Name)
				s.Empty(tool.Lint(), "tool %s follows docs/specs/tools-for-models.md", tool.ToolName())
			}
			for _, prompt := range prompts {
				s.Equal(family.Metadata.Name, prompt.Spec.ToolSet, "prompt %s", prompt.Metadata.Name)
				s.True(strings.HasPrefix(prompt.PromptName(), family.Spec.Prefix+"-"), "prompt %s must carry the %s- prefix", prompt.PromptName(), family.Spec.Prefix)
			}
		})
	}
}

func (s *ExamplesSuite) TestKubeVirtToolsRequireTheVirtualMachineAPI() {
	// The chart must hide its tools while no target serves VirtualMachine,
	// through `requires` on every Tool.
	args := []string{"template", "x", filepath.Join(s.root, "kubevirt-mcp-tools")}
	out, err := exec.Command(s.chartHelm(), args...).CombinedOutput()
	s.Require().NoError(err, string(out))
	count := 0
	for _, doc := range strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n---") {
		if !strings.Contains(doc, "kind: Tool") {
			continue
		}
		var tool Tool
		s.Require().NoError(yaml.Unmarshal([]byte(doc), &tool))
		s.Contains(tool.Spec.Requires, GVKRef{APIVersion: "kubevirt.io/v1", Kind: "VirtualMachine"}, "tool %s must declare the VirtualMachine API in spec.requires", tool.Metadata.Name)
		count++
	}
	s.Positive(count)
}

func TestExamples(t *testing.T) {
	suite.Run(t, new(ExamplesSuite))
}
