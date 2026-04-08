package kubevirt

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

type WindowsGoldenImageSuite struct {
	suite.Suite
}

// mockExamplePipelineRun provides a mock PipelineRun YAML for testing
const mockExamplePipelineRun = `apiVersion: tekton.dev/v1
kind: PipelineRun
metadata:
  generateName: windows-installer-run-
spec:
  params:
  - name: winImageDownloadURL
    value: ""
  - name: acceptEula
    value: "false"
  - name: preferenceName
    value: windows.2k22.virtio
  - name: autounattendConfigMapName
    value: windows2k22-autounattend
  - name: baseDvName
    value: win2k22
  - name: isoDVName
    value: win2k22
  pipelineRef:
    params:
    - name: catalog
      value: kubevirt-tekton-pipelines
    - name: type
      value: artifact
    - name: kind
      value: pipeline
    - name: name
      value: windows-efi-installer
    - name: version
      value: 0.25.0
    resolver: hub
  taskRunSpecs:
  - pipelineTaskName: modify-windows-iso-file
    podTemplate:
      securityContext:
        fsGroup: 107
        runAsUser: 107
  timeouts:
    pipeline: 2h0m0s
`

// Original fetchPipelineRunExample function pointer for restoring after tests
var originalFetchFunc = fetchPipelineRunExample

// mockFetchPipelineRunExample returns the mock example for testing
func mockFetchPipelineRunExample(version string) (string, error) {
	return mockExamplePipelineRun, nil
}

func (s *WindowsGoldenImageSuite) SetupTest() {
	// Replace the fetch function with mock for all tests
	fetchPipelineRunExample = mockFetchPipelineRunExample
}

func (s *WindowsGoldenImageSuite) TearDownTest() {
	// Restore original fetch function
	fetchPipelineRunExample = originalFetchFunc
}

func (s *WindowsGoldenImageSuite) TestPromptRegistration() {
	prompts := initWindowsGoldenImage()
	s.Require().Len(prompts, 1, "Expected 1 prompt")
	s.Equal("windows-golden-image", prompts[0].Prompt.Name)
	s.Equal("Windows Golden Image Creator", prompts[0].Prompt.Title)
	s.Contains(prompts[0].Prompt.Description, "windows-efi-installer")
	s.Len(prompts[0].Prompt.Arguments, 4, "Expected 4 arguments")
	s.NotNil(prompts[0].Handler)
}

func (s *WindowsGoldenImageSuite) TestRequiredArgument() {
	prompts := initWindowsGoldenImage()
	handler := prompts[0].Handler

	s.Run("missing winImageDownloadURL returns error", func() {
		params := api.PromptHandlerParams{
			PromptCallRequest: &mockPromptCallRequest{
				args: map[string]string{},
			},
		}

		result, err := handler(params)
		s.Error(err)
		s.Nil(result)
		s.Contains(err.Error(), "winImageDownloadURL")
	})

	s.Run("empty winImageDownloadURL returns error", func() {
		params := api.PromptHandlerParams{
			PromptCallRequest: &mockPromptCallRequest{
				args: map[string]string{
					"winImageDownloadURL": "",
				},
			},
		}

		result, err := handler(params)
		s.Error(err)
		s.Nil(result)
		s.Contains(err.Error(), "winImageDownloadURL")
	})
}

func (s *WindowsGoldenImageSuite) TestUnsupportedVersion() {
	prompts := initWindowsGoldenImage()
	handler := prompts[0].Handler

	params := api.PromptHandlerParams{
		PromptCallRequest: &mockPromptCallRequest{
			args: map[string]string{
				"winImageDownloadURL": "https://example.com/win.iso",
				"windowsVersion":      "xp",
			},
		},
	}

	result, err := handler(params)
	s.Error(err)
	s.Nil(result)
	s.Contains(err.Error(), "unsupported Windows version")
	s.Contains(err.Error(), "xp")
}

func (s *WindowsGoldenImageSuite) TestWindowsVersionDefaults() {
	tests := []struct {
		version                   string
		preferenceName            string
		autounattendConfigMapName string
		baseDvName                string
		isoDvName                 string
		generateName              string
	}{
		{
			version:                   "10",
			preferenceName:            "windows.10.virtio",
			autounattendConfigMapName: "windows10-autounattend",
			baseDvName:                "win10",
			isoDvName:                 "win10",
			generateName:              "windows10-installer-run-",
		},
		{
			version:                   "11",
			preferenceName:            "windows.11.virtio",
			autounattendConfigMapName: "windows11-autounattend",
			baseDvName:                "win11",
			isoDvName:                 "win11",
			generateName:              "windows11-installer-run-",
		},
		{
			version:                   "2k22",
			preferenceName:            "windows.2k22.virtio",
			autounattendConfigMapName: "windows2k22-autounattend",
			baseDvName:                "win2k22",
			isoDvName:                 "win2k22",
			generateName:              "windows2k22-installer-run-",
		},
		{
			version:                   "2k25",
			preferenceName:            "windows.2k25.virtio",
			autounattendConfigMapName: "windows2k25-autounattend",
			baseDvName:                "win2k25",
			isoDvName:                 "win2k25",
			generateName:              "windows2k25-installer-run-",
		},
	}

	for _, tc := range tests {
		s.Run("version_"+tc.version, func() {
			defaults := resolveWindowsDefaults(tc.version)
			s.Require().NotNil(defaults)
			s.Equal(tc.preferenceName, defaults.preferenceName)
			s.Equal(tc.autounattendConfigMapName, defaults.autounattendConfigMapName)
			s.Equal(tc.baseDvName, defaults.baseDvName)
			s.Equal(tc.isoDvName, defaults.isoDvName)
			s.Equal(tc.generateName, defaults.generateName)
		})
	}

	s.Run("unknown version returns nil", func() {
		defaults := resolveWindowsDefaults("vista")
		s.Nil(defaults)
	})
}

func (s *WindowsGoldenImageSuite) TestDefaultVersion() {
	prompts := initWindowsGoldenImage()
	handler := prompts[0].Handler

	params := api.PromptHandlerParams{
		PromptCallRequest: &mockPromptCallRequest{
			args: map[string]string{
				"winImageDownloadURL": "https://example.com/win.iso",
			},
		},
	}

	result, err := handler(params)
	s.NoError(err)
	s.Require().NotNil(result)

	guideText := result.Messages[0].Content.Text
	s.Contains(guideText, "windows2k22-installer-run-")
	s.Contains(guideText, "windows.2k22.virtio")
	s.Contains(guideText, "windows2k22-autounattend")
}

func (s *WindowsGoldenImageSuite) TestPipelineRunYAML() {
	s.Run("contains correct parameters", func() {
		defaults := resolveWindowsDefaults("2k22")
		yaml, err := buildPipelineRunYAML("https://example.com/win.iso", "test-ns", "", defaults)
		s.Require().NoError(err)

		s.Contains(yaml, "resolver: hub")
		s.Contains(yaml, "value: kubevirt-tekton-pipelines")
		s.Contains(yaml, "value: windows-efi-installer")
		s.Contains(yaml, "value: 0.25.0")
		s.Contains(yaml, "namespace: test-ns")
		s.Contains(yaml, "value: windows.2k22.virtio")
		s.Contains(yaml, "value: windows2k22-autounattend")
		s.Contains(yaml, "value: win2k22")
	})

	s.Run("contains taskRunSpecs with security context", func() {
		defaults := resolveWindowsDefaults("2k22")
		yaml, err := buildPipelineRunYAML("https://example.com/win.iso", "test-ns", "0.25.0", defaults)
		s.Require().NoError(err)

		s.Contains(yaml, "taskRunSpecs:")
		s.Contains(yaml, "pipelineTaskName: modify-windows-iso-file")
		s.Contains(yaml, "fsGroup: 107")
		s.Contains(yaml, "runAsUser: 107")
	})

	s.Run("uses version-specific generateName", func() {
		for version, expected := range map[string]string{
			"10":   "windows10-installer-run-",
			"11":   "windows11-installer-run-",
			"2k22": "windows2k22-installer-run-",
			"2k25": "windows2k25-installer-run-",
		} {
			defaults := resolveWindowsDefaults(version)
			yaml, err := buildPipelineRunYAML("https://example.com/win.iso", "", "", defaults)
			s.Require().NoError(err)
			s.Contains(yaml, "generateName: "+expected)
		}
	})
}

func (s *WindowsGoldenImageSuite) TestExtractPipelineRunFromReadme() {
	s.Run("extracts PipelineRun from plain yaml block", func() {
		readme := "Some text\n\n```yaml\napiVersion: tekton.dev/v1\nkind: PipelineRun\nmetadata:\n  generateName: test-run-\n```\n"
		result, err := extractPipelineRunFromReadme(readme)
		s.Require().NoError(err)
		s.Contains(result, "kind: PipelineRun")
		s.Contains(result, "generateName: test-run-")
	})

	s.Run("extracts PipelineRun from heredoc-wrapped yaml block", func() {
		readme := "```yaml\noc create -f - <<EOF\napiVersion: tekton.dev/v1\nkind: PipelineRun\nmetadata:\n  generateName: heredoc-run-\nEOF\n```\n"
		result, err := extractPipelineRunFromReadme(readme)
		s.Require().NoError(err)
		s.Contains(result, "kind: PipelineRun")
		s.Contains(result, "generateName: heredoc-run-")
		s.NotContains(result, "EOF")
		s.NotContains(result, "oc create")
	})

	s.Run("picks PipelineRun block when multiple yaml blocks exist", func() {
		readme := "```yaml\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: foo\n```\n\n```yaml\napiVersion: tekton.dev/v1\nkind: PipelineRun\nmetadata:\n  generateName: second-run-\n```\n"
		result, err := extractPipelineRunFromReadme(readme)
		s.Require().NoError(err)
		s.Contains(result, "kind: PipelineRun")
		s.Contains(result, "generateName: second-run-")
	})

	s.Run("returns error when no PipelineRun found", func() {
		readme := "```yaml\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: foo\n```\n"
		result, err := extractPipelineRunFromReadme(readme)
		s.Error(err)
		s.Contains(err.Error(), "no PipelineRun example found")
		s.Empty(result)
	})
}

func (s *WindowsGoldenImageSuite) TestSetParams() {
	s.Run("updates existing and appends missing params", func() {
		params := tektonv1.Params{
			{Name: "winImageDownloadURL", Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "old-url"}},
		}
		result := setParams(params, map[string]string{
			"winImageDownloadURL": "new-url",
			"acceptEula":          "false",
		})
		s.Len(result, 2)
		s.Equal("winImageDownloadURL", result[0].Name)
		s.Equal("new-url", result[0].Value.StringVal)

		var found bool
		for _, p := range result {
			if p.Name == "acceptEula" {
				s.Equal("false", p.Value.StringVal)
				found = true
			}
		}
		s.True(found, "acceptEula param should be appended")
	})
}

func (s *WindowsGoldenImageSuite) TestEULAInstructions() {
	prompts := initWindowsGoldenImage()
	handler := prompts[0].Handler

	params := api.PromptHandlerParams{
		PromptCallRequest: &mockPromptCallRequest{
			args: map[string]string{
				"winImageDownloadURL": "https://example.com/win.iso",
			},
		},
	}

	result, err := handler(params)
	s.NoError(err)
	s.Require().NotNil(result)
	s.Require().Len(result.Messages, 2)

	guideText := result.Messages[0].Content.Text
	s.Equal("user", result.Messages[0].Role)
	s.Equal("assistant", result.Messages[1].Role)

	s.Contains(guideText, "EULA", "Guide should mention EULA")
	s.Contains(guideText, "acceptEula", "Guide should reference acceptEula parameter")
	s.Contains(guideText, "Do NOT proceed", "Guide should instruct not to proceed without acceptance")
	s.Contains(guideText, "resources_create_or_update", "Guide should reference the apply tool")
	s.Contains(guideText, "Do you accept the Microsoft EULA", "Guide should contain the acceptance question")
}

func TestWindowsGoldenImage(t *testing.T) {
	suite.Run(t, new(WindowsGoldenImageSuite))
}
