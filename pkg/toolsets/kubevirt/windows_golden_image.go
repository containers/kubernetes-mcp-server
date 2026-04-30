package kubevirt

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
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

// artifactHubPackage represents the Artifact Hub API response for a package
type artifactHubPackage struct {
	Readme string `json:"readme"`
}

const (
	artifactHubAPIBase  = "https://artifacthub.io/api/v1/packages/tekton-pipeline/kubevirt-tekton-pipelines/windows-efi-installer"
	httpTimeout         = 10 * time.Second
	maxResponseBodySize = 512 << 10 // 512 KiB
)

// fetchPipelineRunExample is a variable to allow mocking in tests
var fetchPipelineRunExample = fetchPipelineRunExampleFromArtifactHub

var windowsVersionDefaults = map[string]windowsDefaults{
	"10": {
		preferenceName:            "windows.10.virtio",
		autounattendConfigMapName: "windows10-autounattend",
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
				Title:       "Windows Golden Image Creator",
				Description: "Guides creation of a Windows golden image via the windows-efi-installer Tekton pipeline",
				Arguments: []api.PromptArgument{
					{
						Name:        "winImageDownloadURL",
						Description: "Microsoft Windows ISO download URL",
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

	if windowsVersion == "" {
		windowsVersion = "2k22"
	}

	defaults := resolveWindowsDefaults(windowsVersion)
	if defaults == nil {
		return nil, fmt.Errorf("unsupported Windows version %q: must be one of 10, 11, 2k22, 2k25", windowsVersion)
	}

	pipelineRunYAML, err := buildPipelineRunYAML(winImageDownloadURL, namespace, pipelineVersion, defaults)
	if err != nil {
		return nil, fmt.Errorf("failed to build PipelineRun YAML: %w", err)
	}

	guideText := buildGuideText(pipelineRunYAML)

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
	), nil
}

func resolveWindowsDefaults(version string) *windowsDefaults {
	defaults, ok := windowsVersionDefaults[version]
	if !ok {
		return nil
	}
	return &defaults
}

func buildGuideText(pipelineRunYAML string) string {
	return fmt.Sprintf(`# Windows Golden Image Creator

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

- KubeVirt
- Tekton Pipelines
- Both `+"`kubevirt`"+` and `+"`tekton`"+` toolsets must be enabled
`, pipelineRunYAML)
}

// fetchPipelineRunExampleFromArtifactHub fetches the example PipelineRun YAML from Artifact Hub
func fetchPipelineRunExampleFromArtifactHub(version string) (string, error) {
	url := artifactHubAPIBase
	if version != "" {
		url = fmt.Sprintf("%s/%s", artifactHubAPIBase, version)
	}

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch package from Artifact Hub: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("artifact Hub returned status %d for version %q", resp.StatusCode, version)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var pkg artifactHubPackage
	if err := json.Unmarshal(body, &pkg); err != nil {
		return "", fmt.Errorf("failed to parse Artifact Hub response: %w", err)
	}

	// Extract PipelineRun YAML from README
	// The README contains YAML examples between ```yaml and ``` markers
	example, err := extractPipelineRunFromReadme(pkg.Readme)
	if err != nil {
		return "", fmt.Errorf("failed to extract PipelineRun example: %w", err)
	}

	return example, nil
}

var (
	reCodeBlock   = regexp.MustCompile("(?s)```(?:yaml|YAML)\n(.*?)```")
	rePipelineRun = regexp.MustCompile(`(?s)(apiVersion:.*?kind: PipelineRun.*?)(?:\nEOF\b|$)`)
)

// extractPipelineRunFromReadme extracts the first PipelineRun YAML from a README.
// It handles yaml code blocks that wrap the YAML inside shell heredocs (<<EOF...EOF).
func extractPipelineRunFromReadme(readme string) (string, error) {
	for _, block := range reCodeBlock.FindAllStringSubmatch(readme, -1) {
		if m := rePipelineRun.FindStringSubmatch(block[1]); m != nil {
			return strings.TrimSpace(m[1]) + "\n", nil
		}
	}
	return "", fmt.Errorf("no PipelineRun example found in README")
}

// buildPipelineRunYAML fetches and modifies the PipelineRun template from Artifact Hub
func buildPipelineRunYAML(winImageDownloadURL, namespace, pipelineVersion string, defaults *windowsDefaults) (string, error) {
	exampleYAML, err := fetchPipelineRunExample(pipelineVersion)
	if err != nil {
		return "", err
	}

	var pr tektonv1.PipelineRun
	if err := yaml.Unmarshal([]byte(exampleYAML), &pr); err != nil {
		return "", fmt.Errorf("failed to parse PipelineRun YAML: %w", err)
	}

	pr.TypeMeta = metav1.TypeMeta{APIVersion: "tekton.dev/v1", Kind: "PipelineRun"}
	pr.ObjectMeta = metav1.ObjectMeta{
		GenerateName: defaults.generateName,
		Namespace:    namespace,
	}

	pr.Spec.Params = setParams(pr.Spec.Params, map[string]string{
		"winImageDownloadURL":       winImageDownloadURL,
		"acceptEula":                "false",
		"preferenceName":            defaults.preferenceName,
		"autounattendConfigMapName": defaults.autounattendConfigMapName,
		"baseDvName":                defaults.baseDvName,
		"isoDVName":                 defaults.isoDvName,
	})

	if pipelineVersion != "" && pr.Spec.PipelineRef != nil {
		pr.Spec.PipelineRef.Params = setParams(pr.Spec.PipelineRef.Params, map[string]string{
			"version": pipelineVersion,
		})
	}

	modifiedYAML, err := yaml.Marshal(pr)
	if err != nil {
		return "", fmt.Errorf("failed to marshal modified PipelineRun: %w", err)
	}

	return string(modifiedYAML), nil
}

// setParams updates existing Tekton params in place and appends any missing ones.
func setParams(params tektonv1.Params, values map[string]string) tektonv1.Params {
	seen := make(map[string]bool, len(values))

	for i, p := range params {
		if v, ok := values[p.Name]; ok {
			params[i].Value = tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: v}
			seen[p.Name] = true
		}
	}

	for name, v := range values {
		if !seen[name] {
			params = append(params, tektonv1.Param{
				Name:  name,
				Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: v},
			})
		}
	}

	return params
}
