package kubevirt

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	kubevirt "github.com/containers/kubernetes-mcp-server/pkg/toolsets/kubevirt/internal/defaults"
	pod "github.com/tektoncd/pipeline/pkg/apis/pipeline/pod"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"
)

type windowsDefaults struct {
	preferenceName            string
	autounattendConfigMapName string
	baseDvName                string
	isoDvName                 string
	generateName              string
}

var reVersionFormat = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

var windowsVersionDefaults = map[string]windowsDefaults{
	"10": {
		preferenceName:            "windows.10.virtio",
		autounattendConfigMapName: "windows10-efi-autounattend",
		baseDvName:                "win10",
		isoDvName:                 "win10",
		generateName:              "windows10-installer-run-",
	},
	"11": {
		preferenceName:            "windows.11.virtio",
		autounattendConfigMapName: "windows11-autounattend",
		baseDvName:                "win11",
		isoDvName:                 "win11",
		generateName:              "windows11-installer-run-",
	},
	"2k22": {
		preferenceName:            "windows.2k22.virtio",
		autounattendConfigMapName: "windows2k22-autounattend",
		baseDvName:                "win2k22",
		isoDvName:                 "win2k22",
		generateName:              "windows2k22-installer-run-",
	},
	"2k25": {
		preferenceName:            "windows.2k25.virtio",
		autounattendConfigMapName: "windows2k25-autounattend",
		baseDvName:                "win2k25",
		isoDvName:                 "win2k25",
		generateName:              "windows2k25-installer-run-",
	},
}

func initWindowsGoldenImage() []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Prompt: api.Prompt{
				Name:        "windows-golden-image",
				Title:       fmt.Sprintf("%s Windows Golden Image Creator", kubevirt.ProductName()),
				Description: fmt.Sprintf("Guides creation of a Windows golden image via the %s windows-efi-installer Tekton pipeline", kubevirt.ProductName()),
				Arguments: []api.PromptArgument{
					{
						Name:        "winImageDownloadURL",
						Description: "Microsoft Windows ISO download URL (must be https://)",
						Required:    true,
					},
					{
						Name:        "namespace",
						Description: "Target namespace for the PipelineRun",
						Required:    false,
					},
					{
						Name:        "windowsVersion",
						Description: "Windows version: 10, 11, 2k22 (default), or 2k25",
						Required:    false,
					},
					{
						Name:        "pipelineVersion",
						Description: "Pipeline version (default: latest). Use specific version like 0.25.0 if needed",
						Required:    false,
					},
				},
			},
			Handler:      windowsGoldenImageHandler,
			ClusterAware: ptr.To(false),
		},
	}
}

func windowsGoldenImageHandler(params api.PromptHandlerParams) (*api.PromptCallResult, error) {
	args := params.GetArguments()
	winImageDownloadURL := args["winImageDownloadURL"]
	namespace := args["namespace"]
	windowsVersion := args["windowsVersion"]
	pipelineVersion := args["pipelineVersion"]

	if winImageDownloadURL == "" {
		return nil, fmt.Errorf("winImageDownloadURL argument is required")
	}

	parsedURL, err := url.Parse(winImageDownloadURL)
	if err != nil || parsedURL.Scheme != "https" || parsedURL.User != nil {
		return nil, fmt.Errorf("winImageDownloadURL must be an https:// URL without embedded credentials, got %q", winImageDownloadURL)
	}

	if pipelineVersion != "" && !reVersionFormat.MatchString(pipelineVersion) {
		return nil, fmt.Errorf("invalid pipelineVersion %q: must be in format X.Y.Z (e.g. 0.25.0)", pipelineVersion)
	}

	if windowsVersion == "" {
		windowsVersion = "2k22"
	}

	winDefaults := resolveWindowsDefaults(windowsVersion)
	if winDefaults == nil {
		return nil, fmt.Errorf("unsupported Windows version %q: must be one of 10, 11, 2k22, 2k25", windowsVersion)
	}

	ctx := params.Context
	if ctx == nil {
		ctx = context.Background()
	}

	eulaConsent, err := requestEULAConsent(ctx, params.Elicitor)
	if err != nil {
		return nil, err
	}

	if eulaConsent == eulaElicitationUnsupported {
		pipelineRunYAML, yamlErr := buildStaticPipelineRun(winImageDownloadURL, namespace, pipelineVersion, winDefaults, false)
		if yamlErr != nil {
			return nil, fmt.Errorf("failed to build PipelineRun YAML: %w", yamlErr)
		}
		return buildFallbackGuideResult(pipelineRunYAML), nil
	}

	if eulaConsent == eulaDeclined {
		return api.NewPromptCallResult(
			"Windows golden image creation cancelled",
			[]api.PromptMessage{
				{
					Role: "assistant",
					Content: api.PromptContent{
						Type: "text",
						Text: "The Microsoft EULA was not accepted. The Windows golden image creation has been cancelled.",
					},
				},
			},
			nil,
		), nil
	}

	pipelineRunYAML, err := buildStaticPipelineRun(winImageDownloadURL, namespace, pipelineVersion, winDefaults, true)
	if err != nil {
		return nil, fmt.Errorf("failed to build PipelineRun YAML: %w", err)
	}

	return buildAcceptedGuideResult(pipelineRunYAML), nil
}

type eulaConsentResult int

const (
	eulaAccepted eulaConsentResult = iota
	eulaDeclined
	eulaElicitationUnsupported
)

// requestEULAConsent asks the user to accept the Microsoft EULA via elicitation.
func requestEULAConsent(ctx context.Context, elicitor api.Elicitor) (eulaConsentResult, error) {
	if elicitor == nil {
		return eulaElicitationUnsupported, nil
	}
	result, err := elicitor.Elicit(ctx, &api.ElicitParams{
		Message: "Microsoft EULA Notice: By proceeding, you agree to the Microsoft Software License Terms for the Windows operating system. Do you accept the Microsoft End User License Agreement (EULA)?",
	})
	if err != nil {
		if errors.Is(err, api.ErrElicitationNotSupported) {
			return eulaElicitationUnsupported, nil
		}
		return eulaDeclined, err
	}
	if result.Action == api.ElicitActionAccept {
		return eulaAccepted, nil
	}
	return eulaDeclined, nil
}

func resolveWindowsDefaults(version string) *windowsDefaults {
	winDefaults, ok := windowsVersionDefaults[version]
	if !ok {
		return nil
	}
	return &winDefaults
}

// buildStaticPipelineRun constructs the PipelineRun from a static Go template.
// This avoids network calls, README scraping, and YAML round-trips from Artifact Hub.
func buildStaticPipelineRun(winImageDownloadURL, namespace, pipelineVersion string, winDefaults *windowsDefaults, acceptEula bool) (string, error) {
	eulaStr := "false"
	if acceptEula {
		eulaStr = "true"
	}

	pipelineRefParams := tektonv1.Params{
		{Name: "catalog", Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: kubevirt.WindowsEFIInstallerTektonCatalog()}},
		{Name: "type", Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "artifact"}},
		{Name: "kind", Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "pipeline"}},
		{Name: "name", Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "windows-efi-installer"}},
	}
	if pipelineVersion != "" {
		pipelineRefParams = append(pipelineRefParams, tektonv1.Param{
			Name:  "version",
			Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: pipelineVersion},
		})
	}

	runAs := ptr.To(int64(107))

	pr := tektonv1.PipelineRun{
		TypeMeta: metav1.TypeMeta{APIVersion: "tekton.dev/v1", Kind: "PipelineRun"},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: winDefaults.generateName,
			Namespace:    namespace,
		},
		Spec: tektonv1.PipelineRunSpec{
			Params: tektonv1.Params{
				{Name: "winImageDownloadURL", Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: winImageDownloadURL}},
				{Name: "acceptEula", Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: eulaStr}},
				{Name: "preferenceName", Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: winDefaults.preferenceName}},
				{Name: "autounattendConfigMapName", Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: winDefaults.autounattendConfigMapName}},
				{Name: "baseDvName", Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: winDefaults.baseDvName}},
				{Name: "isoDVName", Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: winDefaults.isoDvName}},
			},
			PipelineRef: &tektonv1.PipelineRef{
				ResolverRef: tektonv1.ResolverRef{
					Resolver: "hub",
					Params:   pipelineRefParams,
				},
			},
			TaskRunSpecs: []tektonv1.PipelineTaskRunSpec{
				{
					PipelineTaskName: "modify-windows-iso-file",
					PodTemplate: &pod.Template{
						SecurityContext: &corev1.PodSecurityContext{
							RunAsUser: runAs,
							FSGroup:   runAs,
						},
					},
				},
			},
			Timeouts: &tektonv1.TimeoutFields{
				Pipeline: &metav1.Duration{Duration: 2 * time.Hour},
			},
		},
	}

	data, err := yaml.Marshal(pr)
	if err != nil {
		return "", fmt.Errorf("failed to marshal PipelineRun: %w", err)
	}
	return string(data), nil
}

func buildAcceptedGuideResult(pipelineRunYAML string) *api.PromptCallResult {
	guideText := fmt.Sprintf(`# %s Windows Golden Image Creator

The user has accepted the Microsoft EULA.

## PipelineRun YAML

The following PipelineRun will:
1. Download the Windows ISO from the provided URL
2. Modify it for EFI automated installation
3. Create a VM and install Windows
4. Produce a bootable DataSource/DataVolume as a golden image

`+"```yaml\n%s```"+`

---

## Steps

### Step 1: Apply the PipelineRun

**You MUST now apply the PipelineRun above using the `+"`resources_create_or_update`"+` tool.**

The EULA has been accepted, and the PipelineRun is ready to be created.

### Step 2: Monitor Progress

After applying the PipelineRun:

1. Use the Tekton tools to monitor the PipelineRun status
2. The pipeline may take up to 2 hours to complete
3. Report progress to the user as the pipeline runs
`, kubevirt.ProductName(), pipelineRunYAML)

	return api.NewPromptCallResult(
		"Windows golden image creation ready",
		[]api.PromptMessage{
			{
				Role: "user",
				Content: api.PromptContent{
					Type: "text",
					Text: guideText,
				},
			},
		},
		nil,
	)
}

func buildFallbackGuideResult(pipelineRunYAML string) *api.PromptCallResult {
	guideText := fmt.Sprintf(`# %s Windows Golden Image Creator

## EULA Notice

**IMPORTANT:** By setting the `+"`acceptEula`"+` parameter to `+"`\"true\"`"+` in the PipelineRun below, you agree to the Microsoft Software License Terms for the Windows operating system.

Before proceeding, you **MUST** ask the user whether they accept the Microsoft End User License Agreement (EULA).

---

## PipelineRun YAML

The following PipelineRun will:
1. Download the Windows ISO from the provided URL
2. Modify it for EFI automated installation
3. Create a VM and install Windows
4. Produce a bootable DataSource/DataVolume as a golden image

`+"```yaml\n%s```"+`

---

## Steps

### Step 1: Ask for EULA Acceptance

Ask the user the following:

> To create the Windows golden image, you must accept the Microsoft End User License Agreement (EULA).
> By proceeding, you agree to the Microsoft Software License Terms for the Windows operating system.
>
> Do you accept the Microsoft EULA? (yes/no)

**Do NOT proceed to Step 2 unless the user explicitly accepts.**
**If the user does not accept, do NOT apply the YAML. Inform them that the operation has been cancelled.**

### Step 2: Apply the PipelineRun

Only after the user has explicitly accepted the EULA:

1. In the YAML above, change `+"`acceptEula`"+` to `+"`\"true\"`"+` (it is currently set to `+"`\"false\"`"+`)
2. Apply the YAML using the `+"`resources_create_or_update`"+` tool

### Step 3: Monitor Progress

After applying the PipelineRun:

1. Use the Tekton tools to monitor the PipelineRun status
2. The pipeline may take up to 2 hours to complete
3. Report progress to the user as the pipeline runs

---

## Prerequisites

- %s
- Tekton Pipelines
- Both `+"`kubevirt`"+` and `+"`tekton`"+` toolsets must be enabled
`, kubevirt.ProductName(), pipelineRunYAML, kubevirt.ProductName())

	return api.NewPromptCallResult(
		"Windows golden image creation guide generated",
		[]api.PromptMessage{
			{
				Role: "user",
				Content: api.PromptContent{
					Type: "text",
					Text: guideText,
				},
			},
			{
				Role: "assistant",
				Content: api.PromptContent{
					Type: "text",
					Text: "I'll help you create a Windows golden image. Before proceeding, I need to ask about the Microsoft EULA acceptance. Let me start with Step 1.",
				},
			},
		},
		nil,
	)
}
