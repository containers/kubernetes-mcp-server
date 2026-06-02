package kubevirt

import (
	"context"
	"fmt"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kubevirtdefaults "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/internal/defaults"
	"github.com/stretchr/testify/suite"
)

type WindowsGoldenImageSuite struct {
	suite.Suite
}

// mockElicitor is a test implementation of api.Elicitor.
type mockElicitor struct {
	action string
	err    error
}

func (m *mockElicitor) Elicit(_ context.Context, _ *api.ElicitParams) (*api.ElicitResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &api.ElicitResult{Action: m.action}, nil
}

func acceptElicitor() *mockElicitor  { return &mockElicitor{action: api.ElicitActionAccept} }
func declineElicitor() *mockElicitor { return &mockElicitor{action: api.ElicitActionDecline} }
func noopElicitor() *mockElicitor {
	return &mockElicitor{err: fmt.Errorf("%w: stub client", api.ErrElicitationNotSupported)}
}

func (s *WindowsGoldenImageSuite) TestPromptRegistration() {
	prompts := initWindowsGoldenImage()
	s.Require().Len(prompts, 1, "Expected 1 prompt")
	s.Equal("windows-golden-image", prompts[0].Prompt.Name)
	s.Equal(fmt.Sprintf("%s Windows Golden Image Creator", kubevirtdefaults.ProductName()), prompts[0].Prompt.Title)
	s.Contains(prompts[0].Prompt.Description, "windows-efi-installer")
	s.Contains(prompts[0].Prompt.Description, kubevirtdefaults.ProductName())
	s.Len(prompts[0].Prompt.Arguments, 4, "Expected 4 arguments")
	s.NotNil(prompts[0].Handler)
}

func (s *WindowsGoldenImageSuite) TestRequiredArgument() {
	handler := initWindowsGoldenImage()[0].Handler

	s.Run("missing winImageDownloadURL returns error", func() {
		params := api.PromptHandlerParams{
			PromptCallRequest: &mockPromptCallRequest{
				args: map[string]string{},
			},
			Elicitor: acceptElicitor(),
		}

		result, err := handler(params)
		s.Error(err)
		s.Nil(result)
		s.Contains(err.Error(), "winImageDownloadURL")
	})
}

func (s *WindowsGoldenImageSuite) TestURLSchemeValidation() {
	handler := initWindowsGoldenImage()[0].Handler

	s.Run("rejects http URL", func() {
		params := api.PromptHandlerParams{
			PromptCallRequest: &mockPromptCallRequest{
				args: map[string]string{"winImageDownloadURL": "http://example.com/win.iso"},
			},
			Elicitor: acceptElicitor(),
		}
		result, err := handler(params)
		s.Error(err)
		s.Nil(result)
		s.Contains(err.Error(), "https://")
	})

	s.Run("rejects URL with no scheme", func() {
		params := api.PromptHandlerParams{
			PromptCallRequest: &mockPromptCallRequest{
				args: map[string]string{"winImageDownloadURL": "example.com/win.iso"},
			},
			Elicitor: acceptElicitor(),
		}
		result, err := handler(params)
		s.Error(err)
		s.Nil(result)
	})

	s.Run("rejects URL with embedded credentials", func() {
		params := api.PromptHandlerParams{
			PromptCallRequest: &mockPromptCallRequest{
				args: map[string]string{"winImageDownloadURL": "https://user:pass@example.com/win.iso"},
			},
			Elicitor: acceptElicitor(),
		}
		result, err := handler(params)
		s.Error(err)
		s.Nil(result)
		s.Contains(err.Error(), "without embedded credentials")
	})

	s.Run("accepts https URL", func() {
		params := api.PromptHandlerParams{
			PromptCallRequest: &mockPromptCallRequest{
				args: map[string]string{"winImageDownloadURL": "https://example.com/win.iso"},
			},
			Elicitor: acceptElicitor(),
		}
		result, err := handler(params)
		s.NoError(err)
		s.NotNil(result)
	})
}

func (s *WindowsGoldenImageSuite) TestUnsupportedVersion() {
	handler := initWindowsGoldenImage()[0].Handler

	params := api.PromptHandlerParams{
		PromptCallRequest: &mockPromptCallRequest{
			args: map[string]string{
				"winImageDownloadURL": "https://example.com/win.iso",
				"windowsVersion":      "xp",
			},
		},
		Elicitor: acceptElicitor(),
	}

	result, err := handler(params)
	s.Error(err)
	s.Nil(result)
	s.Contains(err.Error(), "unsupported Windows version")
	s.Contains(err.Error(), "xp")
}

func (s *WindowsGoldenImageSuite) TestVersionDefaultsInOutput() {
	handler := initWindowsGoldenImage()[0].Handler

	tests := []struct {
		version      string
		preference   string
		configMap    string
		generateName string
	}{
		{"10", "windows.10.virtio", "windows10-efi-autounattend", "windows10-installer-run-"},
		{"11", "windows.11.virtio", "windows11-autounattend", "windows11-installer-run-"},
		{"2k22", "windows.2k22.virtio", "windows2k22-autounattend", "windows2k22-installer-run-"},
		{"2k25", "windows.2k25.virtio", "windows2k25-autounattend", "windows2k25-installer-run-"},
	}

	for _, tc := range tests {
		s.Run("version_"+tc.version, func() {
			params := api.PromptHandlerParams{
				PromptCallRequest: &mockPromptCallRequest{
					args: map[string]string{
						"winImageDownloadURL": "https://example.com/win.iso",
						"windowsVersion":      tc.version,
					},
				},
				Elicitor: acceptElicitor(),
			}

			result, err := handler(params)
			s.Require().NoError(err)
			guideText := result.Messages[0].Content.Text
			s.Contains(guideText, tc.preference)
			s.Contains(guideText, tc.configMap)
			s.Contains(guideText, tc.generateName)
		})
	}
}

func (s *WindowsGoldenImageSuite) TestDefaultVersion() {
	handler := initWindowsGoldenImage()[0].Handler

	params := api.PromptHandlerParams{
		PromptCallRequest: &mockPromptCallRequest{
			args: map[string]string{
				"winImageDownloadURL": "https://example.com/win.iso",
				"namespace":           "test-ns",
			},
		},
		Elicitor: acceptElicitor(),
	}

	result, err := handler(params)
	s.NoError(err)
	s.Require().NotNil(result)

	guideText := result.Messages[0].Content.Text
	s.Contains(guideText, "windows2k22-installer-run-")
	s.Contains(guideText, "windows.2k22.virtio")
	s.Contains(guideText, "windows2k22-autounattend")
	s.Contains(guideText, "resolver: hub")
	s.Contains(guideText, "kubevirt-tekton-pipelines")
	s.Contains(guideText, "windows-efi-installer")
	s.Contains(guideText, "namespace: test-ns")
	s.Contains(guideText, "fsGroup: 107")
	s.Contains(guideText, `value: "true"`, "acceptEula must be true after EULA acceptance")
}

func (s *WindowsGoldenImageSuite) TestVersionParameter() {
	handler := initWindowsGoldenImage()[0].Handler

	s.Run("specific version appears in output YAML", func() {
		params := api.PromptHandlerParams{
			PromptCallRequest: &mockPromptCallRequest{
				args: map[string]string{
					"winImageDownloadURL": "https://example.com/win.iso",
					"pipelineVersion":     "0.25.0",
				},
			},
			Elicitor: acceptElicitor(),
		}

		result, err := handler(params)
		s.Require().NoError(err)
		guideText := result.Messages[0].Content.Text
		s.Contains(guideText, "0.25.0")
	})

	s.Run("rejects version not in X.Y.Z format", func() {
		for _, badVersion := range []string{"latest", "0.25", "../../bad"} {
			s.Run(badVersion, func() {
				params := api.PromptHandlerParams{
					PromptCallRequest: &mockPromptCallRequest{
						args: map[string]string{
							"winImageDownloadURL": "https://example.com/win.iso",
							"pipelineVersion":     badVersion,
						},
					},
					Elicitor: acceptElicitor(),
				}

				result, err := handler(params)
				s.Error(err)
				s.Nil(result)
				s.Contains(err.Error(), "invalid pipelineVersion")
			})
		}
	})
}

func (s *WindowsGoldenImageSuite) TestEULAElicitation() {
	handler := initWindowsGoldenImage()[0].Handler

	baseArgs := map[string]string{
		"winImageDownloadURL": "https://example.com/win.iso",
	}

	s.Run("accepted EULA returns PipelineRun with acceptEula true", func() {
		params := api.PromptHandlerParams{
			PromptCallRequest: &mockPromptCallRequest{args: baseArgs},
			Elicitor:          acceptElicitor(),
		}
		result, err := handler(params)
		s.Require().NoError(err)
		s.Require().Len(result.Messages, 1)
		s.Equal("user", result.Messages[0].Role)
		guideText := result.Messages[0].Content.Text
		s.Contains(guideText, "accepted the Microsoft EULA")
		s.Contains(guideText, "acceptEula")
		s.Contains(guideText, `value: "true"`, "acceptEula param must be true in accepted path")
		s.Contains(guideText, "resources_create_or_update")
		s.Contains(guideText, "You MUST now apply", "Guide must have directive instruction to apply the PipelineRun")
	})

	s.Run("declined EULA returns cancellation message", func() {
		params := api.PromptHandlerParams{
			PromptCallRequest: &mockPromptCallRequest{args: baseArgs},
			Elicitor:          declineElicitor(),
		}
		result, err := handler(params)
		s.Require().NoError(err)
		s.Require().Len(result.Messages, 1)
		s.Equal("assistant", result.Messages[0].Role)
		s.Contains(result.Messages[0].Content.Text, "not accepted")
		s.Contains(result.Messages[0].Content.Text, "cancelled")
	})

	s.Run("falls back to prose guide when elicitation not supported", func() {
		params := api.PromptHandlerParams{
			PromptCallRequest: &mockPromptCallRequest{args: baseArgs},
			Elicitor:          noopElicitor(),
		}
		result, err := handler(params)
		s.Require().NoError(err)
		s.Require().Len(result.Messages, 2)
		s.Equal("user", result.Messages[0].Role)
		guideText := result.Messages[0].Content.Text
		s.Contains(guideText, "EULA")
		s.Contains(guideText, "acceptEula")
		s.Contains(guideText, "Do NOT proceed")
		s.Contains(guideText, "resources_create_or_update")
		s.Contains(guideText, "Do you accept the Microsoft EULA")
		// Fallback guide has acceptEula: "false" and asks agent to change it
		s.Contains(guideText, `"false"`)
	})

	s.Run("falls back to prose guide when elicitor is nil", func() {
		params := api.PromptHandlerParams{
			PromptCallRequest: &mockPromptCallRequest{args: baseArgs},
		}
		result, err := handler(params)
		s.Require().NoError(err)
		s.Require().Len(result.Messages, 2)
		guideText := result.Messages[0].Content.Text
		s.Contains(guideText, "EULA")
		s.Contains(guideText, "Do you accept the Microsoft EULA")
	})
}

func (s *WindowsGoldenImageSuite) TestStaticPipelineRunContents() {
	s.Run("includes hub resolver", func() {
		yaml, err := buildStaticPipelineRun("https://example.com/win.iso", "test-ns", "", &windowsDefaults{
			preferenceName:            "windows.2k22.virtio",
			autounattendConfigMapName: "windows2k22-autounattend",
			baseDvName:                "win2k22",
			isoDvName:                 "win2k22",
			generateName:              "windows2k22-installer-run-",
		}, false)
		s.Require().NoError(err)
		s.Contains(yaml, "resolver: hub")
		s.Contains(yaml, "kubevirt-tekton-pipelines")
		s.Contains(yaml, "windows-efi-installer")
		s.Contains(yaml, "namespace: test-ns")
		s.Contains(yaml, "fsGroup: 107")
		s.Contains(yaml, "runAsUser: 107")
		s.Contains(yaml, "2h")
	})

	s.Run("omits version param when not specified", func() {
		yaml, err := buildStaticPipelineRun("https://example.com/win.iso", "", "", &windowsDefaults{
			generateName: "windows2k22-installer-run-",
		}, false)
		s.Require().NoError(err)
		s.NotContains(yaml, "- name: version")
	})

	s.Run("includes version param when specified", func() {
		yaml, err := buildStaticPipelineRun("https://example.com/win.iso", "", "0.25.0", &windowsDefaults{
			generateName: "windows2k22-installer-run-",
		}, false)
		s.Require().NoError(err)
		s.Contains(yaml, "- name: version")
		s.Contains(yaml, "0.25.0")
	})

	s.Run("sets acceptEula false when not accepted", func() {
		yaml, err := buildStaticPipelineRun("https://example.com/win.iso", "", "", &windowsDefaults{
			generateName: "win-",
		}, false)
		s.Require().NoError(err)
		s.Contains(yaml, "acceptEula")
		s.Contains(yaml, `value: "false"`)
	})

	s.Run("sets acceptEula true when accepted", func() {
		yaml, err := buildStaticPipelineRun("https://example.com/win.iso", "", "", &windowsDefaults{
			generateName: "win-",
		}, true)
		s.Require().NoError(err)
		s.Contains(yaml, `"true"`)
	})

}

func TestWindowsGoldenImage(t *testing.T) {
	suite.Run(t, new(WindowsGoldenImageSuite))
}
